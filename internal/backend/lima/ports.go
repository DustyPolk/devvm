package lima

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
)

// Forwarding ports (ports, ports-all) is emulated, and reviewed: Lima's
// own forwards are off, as every VM's portForwards ignore every guest port
// on every address (noForwardsSet, set where they differ), so Lima
// forwards nothing (its ssh is the VM driver's forward, not a rule).
// devvm's forwarder, which isn't the backend's (vmssh.Forwards), forwards
// the VM's ports instead, over Lima's ssh (ForwardTarget), and ends once
// the VM's hostagent does (its lifeline, ha.pid). Which ports a VM
// forwards is never in lima.yaml.

// ForwardTarget is how devvm's port forwarder reaches a running VM's sshd:
// Lima's ssh port on the host's loopback, as Lima's user, with Lima's own
// key (_config/user in devvm's Lima home), which every VM there
// authorizes.
func (l *machines) ForwardTarget(ctx context.Context, vm string) (backend.ForwardTarget, error) {
	t, err := l.SSH(ctx, vm)
	if err != nil {
		return backend.ForwardTarget{}, err
	}
	if t.User == "" || t.Port <= 0 || t.Host != "127.0.0.1" {
		return backend.ForwardTarget{}, fmt.Errorf("%s: its ssh isn't a user on a host loopback port", vm)
	}
	return backend.ForwardTarget{User: t.User, Key: filepath.Join(l.Dirs.Lima, "_config", "user"), Port: t.Port}, nil
}
