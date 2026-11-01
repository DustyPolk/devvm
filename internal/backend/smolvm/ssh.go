package smolvm

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strconv"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
)

// ssh for editors is emulated, and reviewed: smolvm runs no sshd, so devvm
// runs one of its own in each project VM, for which the caller asks it
// (Confinement.SSH):
//
//   - base has openssh-server, its host keys deleted (sshd.sh), and
//     services.sh makes the VM new ones (ed25519) at each start, before
//     sshd starts: no two VMs made from one pack share them, and devvm
//     reads them through smolvm's exec, as it does on Lima;
//   - sshd runs on devvm's own config (services.sh): the VM's user alone,
//     with the VM's own key (sshKey, vmssh.EnsureKey) alone, no passwords;
//     local forwarding, remote forwards of TCP ports only on 127.77.0.0/16
//     (links' addresses), no remote socket forwards, tunnels or X11. Root
//     in the VM can change that: what keeps the VM's connections from
//     going out through the host is the host's ssh, which asks for none of
//     those but a link's (devvm's entry pins its options);
//   - its guest port, 22, is forwarded from a host loopback port
//     (sshForward): Confinement.SSHPort, or one of the backend's, kept while
//     it stays free, never one of the VM's listed ports. It's the one
//     forward smolvm makes;
//   - SSH answers only once this start's sshd runs (record.SSHUp), so
//     nothing reads the host keys a start is about to replace. A VM whose
//     image has no sshd starts without ssh, warned.
const sshGuestPort = 22

// sshKey is the key devvm's ssh logs in to one VM with, beside its record,
// made at its first start with ssh.
func (s *Smolvm) sshKey(vm string) string {
	return filepath.Join(filepath.Dir(s.Dirs.SmolvmVM(vm)), vm+".key")
}

// sshForward is the host loopback port a VM's ssh is forwarded from at a
// start that runs it: c.SSHPort when the caller chose one (devvm records
// every VM's), else the one it had while that's free, else a new one;
// never one of the VM's listed ports, which its forwarder takes.
func (s *Smolvm) sshForward(ctx context.Context, vm string, c backend.Confinement, r record) (int, error) {
	if c.SSHPort > 0 {
		if lists(r.Listed, c.SSHPort) {
			return 0, msg.SmolvmSSHPortForwarded(strconv.Itoa(c.SSHPort), vm)
		}
		return c.SSHPort, nil
	}
	if r.SSHPort > 0 && !lists(r.Listed, r.SSHPort) && s.PortFree(ctx, r.SSHPort) {
		return r.SSHPort, nil
	}
	for range 20 {
		p, err := s.FreePort(ctx)
		if err != nil {
			return 0, err
		}
		if !lists(r.Listed, p) {
			return p, nil
		}
	}
	return 0, errors.New("smolvm: no free port on the host's loopback")
}

// lists reports whether one of the ranges has the port.
func lists(rs []backend.PortRange, port int) bool {
	for _, r := range rs {
		if r.From <= port && port <= r.To {
			return true
		}
	}
	return false
}

// sshForwardSpec is -p's value for a VM's ssh forward.
func sshForwardSpec(port int) string { return strconv.Itoa(port) + ":" + strconv.Itoa(sshGuestPort) }

// freePort is a port free on the host's loopback, IPv4 and IPv6, as
// smolvm forwards on both, and on every other address
// (platform.PortFree).
func freePort(ctx context.Context) (int, error) {
	for range 20 {
		l, err := platform.ListenLoopback(ctx, "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if platform.PortFree(ctx, port) {
			return port, nil
		}
	}
	return 0, errors.New("smolvm: no free port on the host's loopback")
}

// servicesArgs are services.sh's arguments for a start that runs the VM's
// sshd: the user, devvm's public key, and the names of the variables its
// sessions get: the proxy's under network policy, then env's.
func servicesArgs(user, pub string, env map[string]string, proxy bool) []string {
	var first []string
	if proxy {
		first = proxyVars
	}
	return vmssh.ServicesArgs(user, pub, first, env)
}

// SSH returns how ssh reaches a running VM whose start ran its sshd: its
// forward on the host's loopback, the VM's user, and devvm's key alone.
// A VM started without it gets vm_no_ssh.
func (s *Smolvm) SSH(ctx context.Context, vm string) (backend.SSHTarget, error) {
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return backend.SSHTarget{}, notFound(vm, err)
	}
	if m.status() != backend.StatusRunning {
		return backend.SSHTarget{}, msg.VMNotRunning(vm)
	}
	r, err := s.readRecord(vm)
	if err != nil {
		return backend.SSHTarget{}, err
	}
	switch {
	case r.NoSSHD:
		return backend.SSHTarget{}, msg.VMNoSshd(vm)
	case r.SSHPort == 0 || !r.SSHUp:
		return backend.SSHTarget{}, msg.VMNoSSH(vm)
	}
	name, _, err := s.user()
	if err != nil {
		return backend.SSHTarget{}, err
	}
	return backend.SSHTarget{
		Host: "127.0.0.1", Port: r.SSHPort, User: name,
		Options: []backend.SSHOption{
			{Key: "IdentityFile", Value: `"` + s.sshKey(vm) + `"`},
			{Key: "IdentitiesOnly", Value: "yes"},
			{Key: "PreferredAuthentications", Value: "publickey"},
		},
	}, nil
}
