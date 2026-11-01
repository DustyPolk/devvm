package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newRestartCmd is devvm restart.
func newRestartCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart this folder's VM and open a shell in it",
		Long: "Stop this folder's VM, then start it and open a shell in it as devvm does. " +
			"Changes that apply only when a VM starts, such as a new size or an agent's folders, apply then.",
		Example: examples(
			"devvm restart", "restart this folder's VM and open a shell in it",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Launch{Env: env}.Restart(cmd.Context(), vm.Options{}) }, msg.UpFailed)
			return nil
		},
	}
}
