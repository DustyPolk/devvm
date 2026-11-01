package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newRmCmd is devvm rm [--force] [--no-export].
func newRmCmd(u *ui.UI) *cobra.Command {
	var o vm.RmOptions
	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Delete this folder's VM",
		Long: "Delete this folder's VM and what devvm keeps for it, after asking in a terminal. " +
			"When its image has an agent, devvm first exports the agents' history, unless you give --no-export. " +
			"A --clone VM's work that this repo hasn't fetched is offered for fetching first.",
		Example: examples(
			"devvm rm", "delete this folder's VM, after asking",
			"devvm rm --force", "delete it without asking, as a script must",
			"devvm rm --no-export", "delete it without exporting the agents' history",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Rm(cmd.Context(), o) }, msg.RmFailed)
			return nil
		},
	}
	addForceFlag(cmd.Flags(), &o.Force)
	cmd.Flags().BoolVar(&o.NoExport, "no-export", false, "delete without exporting the agents' history first")
	return cmd
}
