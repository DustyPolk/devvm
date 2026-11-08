package cli

import (
	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/vm"
)

// newRestartCmd is devvm restart.
func newRestartCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use: "restart",
		Annotations: about{
			docs: "using/vms#restarting-and-stopping", group: groupVM,
			noJSON: "it restarts the VM",
		}.annotations(),
		Short: "Restart this folder's VM and open a shell in it",
		Long: "Stop this folder's VM, then start it and open a shell in it as devvm does. " +
			"Changes that apply only when a VM starts, such as a new size or an agent's folders, apply then.",
		Example: examples(
			"devvm restart", "restart this folder's VM and open a shell in it",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.env(func(env *vm.Env) error { return vm.Launch{Env: env}.Restart(cmd.Context(), vm.Options{}) }, msg.UpFailed)
		},
	}
	addFolderFlags(cmd, a)
	return cmd
}
