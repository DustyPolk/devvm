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

// newSmolvmPortsCmd is devvm _smolvm-ports: the port forwarder of a smolvm
// VM that forwards every port, which the backend starts detached. Hidden:
// it's not for users. Its flags are the forwarder's own, parsed by
// smolvm.ForwardMain; its log lines are data, written to devvm's stderr.
// It never forwards a port devvm records for a VM (netpolicy.UsedPorts).
func newSmolvmPortsCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:                "_smolvm-ports [flags]",
		Short:              "Forward a smolvm VM's ports",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fail := func(err error) { u.Die(msg.SmolvmForwarderFailed(err.Error()).Because(err)) }
			d, err := paths.FromEnv()
			if err != nil {
				fail(err)
			}
			script, err := fs.ReadFile(assets.FS, "smolvm/ports.sh")
			if err != nil {
				fail(err)
			}
			reserved := func() (map[int]bool, error) { return netpolicy.UsedPorts(d) }
			if err := smolvm.ForwardMain(cmd.Context(), args, script, reserved, u.Stderr()); err != nil {
				fail(err)
			}
			return nil
		},
	}
}
