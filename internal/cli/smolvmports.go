package cli

import (
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// forwarderMain runs a backend's port forwarder (main) with its
// arguments, ports.sh and the ports devvm records for VMs, which it never
// forwards (netpolicy.UsedPorts) but its VM's own listed ones; its log
// lines are data, written to devvm's stderr.
type forwarderMain func(args []string, script []byte, reserved func(vm string) (map[int]bool, error)) error

// newForwarderCmd is a backend's hidden port forwarder command, which the
// backend starts detached: use, short and run. Hidden: it's not for users.
// Its flags are the forwarder's own, parsed by run.
func newForwarderCmd(u *ui.UI, use, short string, run func(cmd *cobra.Command) forwarderMain) *cobra.Command {
	return &cobra.Command{
		Use:                use + " [flags]",
		Short:              short,
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fail := func(err error) { u.Die(msg.ForwarderFailed(err.Error()).Because(err)) }
			d, err := paths.FromEnv()
			if err != nil {
				fail(err)
			}
			script, err := fs.ReadFile(assets.FS, "container/ports.sh")
			if err != nil {
				fail(err)
			}
			reserved := func(vm string) (map[int]bool, error) { return netpolicy.UsedPorts(d, vm) }
			if err := run(cmd)(args, script, reserved); err != nil {
				fail(err)
			}
			return nil
		},
	}
}

// newSmolvmPortsCmd is devvm _smolvm-ports: the port forwarder of a smolvm
// VM that forwards every port (smolvm.ForwardMain).
func newSmolvmPortsCmd(u *ui.UI) *cobra.Command {
	return newForwarderCmd(u, "_smolvm-ports", "Forward a smolvm VM's ports", func(cmd *cobra.Command) forwarderMain {
		return func(args []string, script []byte, reserved func(string) (map[int]bool, error)) error {
			return smolvm.ForwardMain(cmd.Context(), args, script, reserved, u.Stderr())
		}
	})
}
