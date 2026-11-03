package lima

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
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
// SetPorts, which starts a running VM's forwarder again: never a lima.yaml
// edit.

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_lima-ports"

// Ports records' values besides a list (5173,8000-8010).
const (
	portsAll  = "all"
	portsNone = "none"
)

// forwards is how Lima's VMs' ports are forwarded: each VM's record of
// which ports it forwards, and its forwarder, their files in its backend
// folder.
type forwards struct {
	vmssh.Forwards
	// Key is Lima's own ssh key, which the forwarder logs in with.
	Key string
}

// newForwards is how Lima's VMs' ports are forwarded: devvm _lima-ports,
// self, their files in each VM's backend folder.
func newForwards(d paths.Dirs, self string) forwards {
	return forwards{
		Forwards: vmssh.Forwards{
			Dirs: d, Prefix: "lima", Cmd: forwarderCmd, Self: self,
			Detach: daemon.Detach, Args: platform.ProcessArgs, Tick: 500 * time.Millisecond,
		},
		Key: filepath.Join(d.Lima, "_config", "user"),
	}
}

// write records which ports a VM forwards (paths.VM.Ports).
func (f forwards) write(vm string, p backend.Ports) error {
	v := portsNone
	switch {
	case p.All:
		v = portsAll
	case len(p.Ranges) > 0:
		v = backend.FormatPortRanges(p.Ranges)
	}
	return state.WriteFile(f.Dirs.VM(vm).Ports(), []byte(v+"\n"))
}

// read returns which ports a VM forwards, as its record has them. A VM
// without a record forwards none (an error): CreateVM writes one.
func (f forwards) read(vm string) (backend.Ports, error) {
	b, err := os.ReadFile(f.Dirs.VM(vm).Ports())
	if err != nil {
		return backend.Ports{}, err
	}
	switch v := strings.TrimSpace(string(b)); v {
	case portsAll:
		return backend.Ports{All: true}, nil
	case portsNone:
		return backend.Ports{}, nil
	default:
		rs, err := backend.ParsePortRanges(v)
		return backend.Ports{Ranges: rs}, err
	}
}

// start starts the forwarder of a project VM that forwards ports, once it
// runs, after stopping any it had, holding its lock throughout: its
// record's ports (c.Forwarded), over Lima's ssh (ssh), with the host key
// devvm pins for it, never on its ssh's or proxy's port, following its
// hostagent's pid file (lifeline). It reports whether it did: one that
// can't start is warned about by the caller, and the VM runs on. A VM that
// forwards none is left without one.
func (f forwards) start(ctx context.Context, vm string, c backend.Confinement, ssh func(context.Context, string) (backend.SSHTarget, error), lifeline string) bool {
	unlock, err := f.Lock(ctx, vm)
	if err != nil {
		return false
	}
	defer unlock()
	f.Stop(vm)
	p, err := f.read(vm)
	if err != nil {
		return false
	}
	only := c.Forwarded(ctx, p)
	if only != nil && len(only) == 0 {
		return true
	}
	if c.HostKey == nil {
		return false
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

// ForwardMain runs a VM's forwarder (devvm _lima-ports) with its
// arguments (vmssh.LifelineMain): its lifeline is the VM's hostagent's
// pid file, its port Lima's ssh's.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	return vmssh.LifelineMain(ctx, forwarderCmd, args, script, reserved, log)
}

// SetPorts changes which of a VM's ports are forwarded (every one, its
// listed ones, or none): its record, which a stopped VM forwards from its
// next start, and a running one's forwarder, started again with them.
func (l *Lima) SetPorts(ctx context.Context, vm string, p backend.Ports, c backend.Confinement) error {
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return err
	}
	if err := l.Forwards.write(vm, p); err != nil {
		return err
	}
	if backend.Status(in.Status) == backend.StatusRunning && !l.Forwards.start(ctx, vm, c, l.SSH, l.haPIDFile(vm)) {
		l.Warn(msg.PortsForwardFailed(l.Forwards.Log(vm), vm))
	}
	return nil
}
