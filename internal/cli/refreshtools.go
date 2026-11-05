package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/tools"
)

// newRefreshToolsCmd is devvm _refresh-tools: the daily check for newer
// shared tools, which devvm starts in the background (tools.RefreshDaily).
// Hidden: it's not for users.
func newRefreshToolsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:    "_refresh-tools",
		Short:  "Resolve the shared tools' versions again",
		Hidden: true,
		Args:   noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			t, err := a.tools()
			if err == nil {
				err = tools.Versions{Env: t}.Refresh(cmd.Context())
			}
			return failed(err, func(string) msg.M { return msg.ToolsCheckFailed() })
		},
	}
}
