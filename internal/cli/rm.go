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
		Long: "Delete this folder's VM, with what devvm keeps for it: its state, its own allowlist, its " +
			"network log, its .git locks or the repo's remote for its clone, and its ssh config entry. " +
			"It asks first in a terminal; --force deletes without asking, which it needs without a terminal. " +
			"When its image has an agent, its agents' history is exported first, unless --no-export, and a " +
			"failed export keeps the VM. A --clone VM's branches this repo hasn't fetched are offered for " +
			"fetching, and its uncommitted changes deleted only after you confirm. A VM made for an earlier " +
			"folder at this path is deleted only after you confirm, and in a folder without a VM of its own, " +
			"it's the VM of the nearest folder with one that this folder is inside, which devvm names and " +
			"deletes only after you confirm. These confirmations are asked only in a terminal, and --force " +
			"never answers them.",
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
