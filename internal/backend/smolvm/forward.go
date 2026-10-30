package smolvm

import (
	"context"
	"io"

	"github.com/w3cj/devvm/internal/backend/vmssh"
)

// Forwarding every port (ports-all) is emulated, and reviewed: smolvm
// forwards only the ports a machine was made or updated with, at most 64,
// and only to servers listening on all of the guest's addresses. So a VM
// that forwards every port gets none of smolvm's, and devvm's forwarder
// instead (vmssh.Forwarder, devvm _smolvm-ports, started by Start once its
// sshd runs): it reaches the VM's sshd on its forward on the host's
// loopback, and ends when the machine's process does (its lifeline,
// agent.pid).

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_smolvm-ports"

// ForwardMain runs a VM's forwarder (devvm _smolvm-ports) with its
// arguments: ports.sh is script, reserved the ports devvm records for VMs
// (but the VM's own listed ones), and its lines go to log. Besides the
// forwarder's own flags, it takes the machine's lifeline and the host port
// its sshd is forwarded from.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	return vmssh.LifelineMain(ctx, forwarderCmd, args, script, reserved, log)
}
