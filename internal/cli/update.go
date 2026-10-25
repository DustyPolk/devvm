package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newUpdateCmd is devvm update.
func newUpdateCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Move this folder's VM to the newest shared tools, then open a shell in it",
		Long: "Check for newer versions of the shared tools (node, python, rust, go, the agents), build " +
			"them, and move this folder's VM to them, then open a shell in it. Nothing in the VM's disk " +
			"changes: its tools are mounted from the share.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return env.Update(cmd.Context(), g.options()) }, msg.UpFailed)
			return nil
		},
	}
}
