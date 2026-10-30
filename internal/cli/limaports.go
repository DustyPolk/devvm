package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/ui"
)

// newLimaPortsCmd is devvm _lima-ports: the port forwarder of a Lima VM
// that forwards ports (lima.ForwardMain).
func newLimaPortsCmd(u *ui.UI) *cobra.Command {
	return newForwarderCmd(u, "_lima-ports", "Forward a Lima VM's ports", func(cmd *cobra.Command) forwarderMain {
		return func(args []string, script []byte, reserved func(string) (map[int]bool, error)) error {
			return lima.ForwardMain(cmd.Context(), args, script, reserved, u.Stderr())
		}
	})
}
