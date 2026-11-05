package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newUpdateCmd is devvm update [--force] [--no-export] [--clean-home].
func newUpdateCmd(a *app) *cobra.Command {
	var o vm.RemakeOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Bring this folder's VM up to date, then open a shell in it",
		Long: "Move this folder's VM to the newest shared tools, as devvm tools update does, then open a shell in it. " +
			"When its image changed since devvm made the VM, devvm then asks to make the VM again from the image's new build, with the same layers, keeping its home folder, the project folder and its settings, and everything else on its disk is lost. " +
			"When its image has an agent, devvm first exports the agents' history, unless you give --no-export.",
		Example: examples(
			"devvm update", "move the VM to the newest tools, and to its image's new build after asking",
			"devvm update --force", "the same without asking, as a script must",
			"devvm update --clean-home", "start from the image's home folder too",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.env(func(env *vm.Env) error {
				return vm.Lifecycle{Env: env}.Update(cmd.Context(), "devvm update", o)
			}, msg.RemakeFailed)
			return nil
		},
	}
	addRemakeFlags(cmd.Flags(), &o)
	return cmd
}
