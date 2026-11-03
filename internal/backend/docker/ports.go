package docker

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
)

// Forwarding ports (ports, ports-all) is emulated, and reviewed: a
// container's ports reach the host only when it publishes them, which
// Docker fixes when it makes the container, and then only for servers on
// the container's network address, never its loopback. So a VM publishes
// nothing, and devvm's forwarder (vmssh.Forwarder, devvm _docker-ports,
// started by Start once the VM's sshd config is written, and again by
// SetPorts on a running VM) forwards its ports over its ssh (ssh.go):
// every one, or its listed ones less those a program on the host has
// (Confinement.Free). It reaches the VM's sshd
// through devvm _docker-connect, as ssh does, and ends once the container
// no longer runs the start it was made for: its ID and start time, asked
// of Docker every aliveEvery and whenever its connection ends (which a
// stopped container ends).

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_docker-ports"

// aliveEvery is how often a forwarder asks Docker whether its container
// still runs.
const aliveEvery = 30 * time.Second

// newForwards is how Docker's VMs' forwarders run: devvm _docker-ports,
// self, their files in each VM's backend folder.
func newForwards(d paths.Dirs, self string) vmssh.Forwards {
	return vmssh.Forwards{
		Dirs: d, Prefix: "docker", Cmd: forwarderCmd, Self: self,
		Detach: daemon.Detach, Args: platform.ProcessArgs, Tick: 500 * time.Millisecond,
	}
}

// forward starts the forwarder of a VM that forwards ports, once its
// sshd's config is written, after stopping any it had, holding its lock
// throughout: every port or its listed ones (its record's as it reads them
// then, c.Forwarded), with the host key this start pinned, through devvm
// _docker-connect, for this start of its container (ct). One that can't
// start is warned about: the VM runs on. A VM that forwards none is left
// without one.
func (d *Docker) forward(ctx context.Context, vm string, ct container, r record, c backend.Confinement, hostKey string) {
	failed := func() { d.Warn(msg.PortsForwardFailed(d.Forwards.Log(vm), vm)) }
	unlock, err := d.Forwards.Lock(ctx, vm)
	if err != nil {
		failed()
		return
	}
	defer unlock()
	d.Forwards.Stop(vm)
	// Another devvm's SetPorts may have changed the ports since r was read.
	if cur, err := d.readRecord(vm); err == nil {
		r.PortsAll, r.Ports = cur.PortsAll, cur.Ports
	}
	only := c.Forwarded(ctx, backend.Ports{All: r.PortsAll, Ranges: r.Ports})
	if only != nil && len(only) == 0 {
		return
	}
	if !r.SSHUp {
		d.Warn(msg.PortsNoSSH(vm))
		return
	}
	t, err := d.transport(vm)
	docker, lerr := d.LookPath("docker")
	if err != nil || lerr != nil {
		failed()
		return
	}
	spec := vmssh.Spec{
		User: t.User, Key: t.Key, HostKey: hostKey, Only: only,
		Args: []string{"-docker", docker, "-container", ct.ID, "-started", ct.State.StartedAt, "-proxy", t.Proxy},
	}
	if d.Forwards.Start(vm, spec) != nil {
		failed()
	}
}

// ForwardMain runs a VM's forwarder (devvm _docker-ports) with its
// arguments: ports.sh is script, reserved the ports devvm records for VMs
// (but the VM's own listed ones), and its lines go to log. Besides the
// forwarder's own flags, it takes docker's path, the container's ID and
// start time, and ssh's ProxyCommand for the VM.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	fs := flag.NewFlagSet(forwarderCmd, flag.ContinueOnError)
	fs.SetOutput(log)
	var fl vmssh.Flags
	var docker, id, started string
	fl.Register(fs)
	fs.StringVar(&docker, "docker", "", "docker's path")
	fs.StringVar(&id, "container", "", "the VM's container's ID: exit once it doesn't run its start")
	fs.StringVar(&started, "started", "", "the container's start time, as Docker has it")
	fs.StringVar(&fl.Proxy, "proxy", "", "ssh's ProxyCommand for the VM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if docker == "" || id == "" || started == "" {
		return errors.New("-docker, -container and -started are needed")
	}
	f, err := fl.Forwarder(script, reserved, log)
	if err != nil {
		return err
	}
	f.Alive = func(ctx context.Context) bool { return containerRuns(ctx, docker, id, started) }
	f.Every = aliveEvery
	return f.Run(ctx)
}

// containerRuns reports whether Docker runs the container by that ID,
// started at that time.
func containerRuns(ctx context.Context, docker, id, started string) bool {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, docker, "container", "inspect", "--format", "{{.State.Running}} {{.State.StartedAt}}", id).Output() //nolint:gosec // docker, as devvm found it
	return err == nil && strings.TrimSpace(string(out)) == "true "+started
}

// SetPorts changes which of a VM's ports are forwarded (every one, its
// listed ones, or none): its record, which a stopped VM forwards from its
// next start, and a running one's forwarder, started again with them and
// the host key its start pinned. One started outside devvm since has no
// sshd of devvm's to forward through.
func (d *Docker) SetPorts(ctx context.Context, vm string, p backend.Ports, c backend.Confinement) error {
	ct, err := d.vm(ctx, vm)
	if err != nil {
		return err
	}
	r, err := d.readRecord(vm)
	if err != nil {
		return err
	}
	if r.PortsAll != p.All || !slices.Equal(r.Ports, p.Ranges) {
		r.PortsAll, r.Ports = p.All, slices.Clone(p.Ranges)
		if err := d.writeRecord(vm, r); err != nil {
			return err
		}
	}
	if ct.status() != backend.StatusRunning {
		return nil
	}
	b, _ := os.ReadFile(d.knownHosts(vm))
	key, ok := vmssh.HostKey(bytes.TrimPrefix(b, []byte(vm+" ")))
	if !ok || r.Started != ct.State.StartedAt {
		r.SSHUp = false
	}
	d.forward(ctx, vm, ct, r, c, key)
	return nil
}
