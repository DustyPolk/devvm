package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// newRefreshToolsCmd is devvm _refresh-tools: the daily check for newer
// shared tools, which devvm starts in the background (tools.RefreshDaily).
// Hidden: it's not for users.
func newRefreshToolsCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:    "_refresh-tools",
		Short:  "Resolve the shared tools' versions again",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirs, err := paths.FromEnv()
			if err == nil {
				var root string
				if root, err = paths.Root(); err == nil {
					err = tools.New(dirs, root, u).Refresh(cmd.Context())
				}
			}
			fail(u, err, func(string) msg.M { return msg.ToolsCheckFailed() })
			return nil
		},
	}
}
