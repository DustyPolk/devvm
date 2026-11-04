package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newUpdateCmd is devvm update [--force] [--no-export] [--clean-home].
func newUpdateCmd(u *ui.UI) *cobra.Command {
	var o vm.RemakeOptions
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Make this folder's VM again from its image's new build, then open a shell in it",
		Long: "Make this folder's VM again from its image's new build, with the same layers, when that image changed since devvm made the VM, then open a shell in it, after asking in a terminal. " +
			"devvm keeps its home folder, the project folder and its settings, and everything else on its disk is lost. " +
			"When its image has an agent, devvm first exports the agents' history, unless you give --no-export.",
		Example: examples(
			"devvm update", "make this folder's VM again from its image's new build, after asking",
			"devvm update --force", "the same without asking, as a script must",
			"devvm update --clean-home", "start from the image's home folder too",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error {
				return vm.Lifecycle{Env: env}.Update(cmd.Context(), "devvm update", o)
			}, msg.RemakeFailed)
			return nil
		},
	}
	addRemakeFlags(cmd.Flags(), &o)
	return cmd
}
