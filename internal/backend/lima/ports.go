package lima

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Forwarding ports (ports, ports-all) is emulated, and reviewed, so every
// backend forwards by the same rules. Lima's own forwards are off: every
// VM's portForwards ignore every guest port on every address
// (noForwardsSet, set where they differ), so Lima forwards nothing (its
// ssh is the VM driver's forward, not a rule). devvm's forwarder
// (vmssh.Forwarder, devvm _lima-ports) forwards the VM's ports instead:
// started by Start once a project VM runs, over Lima's ssh with Lima's key
// and the host key devvm pins for the VM (Confinement.HostKey), and ending
// once the VM's hostagent does (its lifeline, ha.pid); Stop and Delete
// stop it. Which ports a VM forwards (every one, its listed ones, none) is
// its record, <vm>.ports beside the forwarder's files, set by CreateVM and
// SetPorts: never a lima.yaml edit.

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_lima-ports"

// Ports records' values besides a list (5173,8000-8010).
const (
	portsAll  = "all"
	portsNone = "none"
)

// forwards is how Lima's VMs' ports are forwarded: each VM's record of
// which ports it forwards, and its forwarder, their files in Dir.
type forwards struct {
	vmssh.Forwards
	// Key is Lima's own ssh key, which the forwarder logs in with.
	Key string
}

// newForwards is how Lima's VMs' ports are forwarded: devvm _lima-ports,
// self, their files in LimaVMs.
func newForwards(d paths.Dirs, self string) forwards {
	return forwards{
		Forwards: vmssh.Forwards{
			Dir: d.LimaVMs(), Run: d.Run(), Prefix: "lima", Cmd: forwarderCmd, Self: self,
			Detach: platform.StartDetached, Args: platform.ProcessArgs, Tick: 500 * time.Millisecond,
		},
		Key: filepath.Join(d.Lima, "_config", "user"),
	}
}

// file is the record of which ports a VM forwards.
func (f forwards) file(vm string) string { return filepath.Join(f.Dir, vm+".ports") }

// write records which ports a VM forwards.
func (f forwards) write(vm string, p backend.Ports) error {
	v := portsNone
	switch {
	case p.All:
		v = portsAll
	case len(p.Ranges) > 0:
		v = vmssh.FormatRanges(p.Ranges)
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	return state.WriteFile(f.file(vm), []byte(v+"\n"))
}

// read returns which ports a VM forwards, as its record has them. A VM
// without a record forwards none (an error): CreateVM writes one.
func (f forwards) read(vm string) (backend.Ports, error) {
	b, err := os.ReadFile(f.file(vm))
	if err != nil {
		return backend.Ports{}, err
	}
	switch v := strings.TrimSpace(string(b)); v {
	case portsAll:
		return backend.Ports{All: true}, nil
	case portsNone:
		return backend.Ports{}, nil
	default:
		rs, err := vmssh.ParseRanges(v)
		return backend.Ports{Ranges: rs}, err
	}
}

// only is what a VM forwards from this start: nil for every port, else
// its listed ports c names (those a host program has left out), or its
// record's without them; none is empty.
func (f forwards) only(vm string, c backend.Confinement) ([]backend.PortRange, error) {
	if c.Ports != nil {
		return append([]backend.PortRange{}, c.Ports.Ranges...), nil
	}
	p, err := f.read(vm)
	if err != nil || p.All {
		return nil, err
	}
	return append([]backend.PortRange{}, p.Ranges...), nil
}

// start starts the forwarder of a project VM that forwards ports, once it
// runs: over Lima's ssh (ssh), with the host key devvm pins for it, never
// on its ssh's or proxy's port, following its hostagent's pid file
// (lifeline). It reports whether it did: one that can't start is warned
// about by the caller, and the VM runs on. A VM that forwards none has its
// forwarder, if any, stopped.
func (f forwards) start(ctx context.Context, vm string, c backend.Confinement, ssh func(context.Context, string) (backend.SSHTarget, error), lifeline string) bool {
	only, err := f.only(vm, c)
	if err != nil {
		return false
	}
	if only != nil && len(only) == 0 {
		f.Stop(vm)
		return true
	}
	key, err := c.HostKey(ctx)
	t, terr := ssh(ctx, vm)
	if err != nil || terr != nil || t.User == "" || t.Port <= 0 {
		return false
	}
	spec := vmssh.Spec{
		User: t.User, Key: f.Key, HostKey: key, Reserve: []int{t.Port, c.ProxyPort}, Only: only,
		Args: []string{"-lifeline", lifeline, "-port", strconv.Itoa(t.Port)},
	}
	return f.Start(vm, spec) == nil
}

// forget stops the VM's forwarder and deletes its files and its record.
func (f forwards) forget(vm string) error {
	if err := f.Remove(vm); err != nil {
		return err
	}
	if err := os.Remove(f.file(vm)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ForwardMain runs a VM's forwarder (devvm _lima-ports) with its
// arguments (vmssh.LifelineMain): its lifeline is the VM's hostagent's
// pid file, its port Lima's ssh's.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	return vmssh.LifelineMain(ctx, forwarderCmd, args, script, reserved, log)
}

// SetPorts changes which of a stopped VM's ports are forwarded, from its
// next start: every one, its listed ones, or none.
func (l *Lima) SetPorts(ctx context.Context, vm string, p backend.Ports) error {
	if _, err := l.stopped(ctx, vm); err != nil {
		return err
	}
	return l.Forwards.write(vm, p)
}
