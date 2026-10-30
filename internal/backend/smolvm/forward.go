package smolvm

import (
	"context"
	"io"

	"github.com/w3cj/devvm/internal/backend/vmssh"
)

// Forwarding ports (ports, ports-all) is emulated, and reviewed, so every
// backend forwards by the same rules: smolvm forwards only the ports a
// machine was made or updated with, at most 64, only to servers listening
// on all of the guest's addresses, and shares a port a host program has
// (SO_REUSEADDR). So a VM's ports get none of smolvm's forwards, and
// devvm's forwarder instead (vmssh.Forwarder, devvm _smolvm-ports, started
// by Start once its sshd runs): every port, or its listed ones less those
// a host program has (Confinement.Ports). It reaches the VM's sshd on its
// forward on the host's loopback, smolvm's one forward, and ends when the
// machine's process does (its lifeline, agent.pid). Which ports a VM
// forwards is its record's (PortsAll, Listed), set by CreateVM and
// SetPorts, which edit no forward of smolvm's.

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
