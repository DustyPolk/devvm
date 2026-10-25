package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newRmCmd is devvm rm [-f].
func newRmCmd(u *ui.UI) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Delete this folder's VM",
		Long: "Delete this folder's VM, with what devvm keeps for it: its state, its own allowlist, its " +
			"network log, its .git locks or the repo's remote for its clone, and its ssh config entry. " +
			"When its image has an agent, its agents' history is exported first, and a failed export " +
			"keeps the VM. A --clone VM's branches this repo hasn't fetched are offered for fetching, " +
			"and its uncommitted changes deleted only after you confirm. A VM made for an earlier " +
			"folder at this path is deleted only after you confirm, even with -f.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Rm(cmd.Context(), force) }, msg.RmFailed)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "delete without exporting the agents' history or checking a clone for unfetched commits and uncommitted changes")
	return cmd
}
