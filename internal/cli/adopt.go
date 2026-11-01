package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newAdoptCmd is devvm adopt [--force].
func newAdoptCmd(u *ui.UI) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Use the VM made for an earlier folder at this path here",
		Long: "Give this folder the VM made for an earlier folder at the same path, with its files, secrets and allowlist. " +
			"devvm refuses that VM in a folder made again, until you adopt it or delete it with devvm rm. " +
			"devvm asks first in a terminal.",
		Example: examples(
			"devvm adopt", "give this folder the VM, after asking",
			"devvm adopt --force", "give it without asking, as a script must",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Adopt(cmd.Context(), force) }, msg.AdoptFailed)
			return nil
		},
	}
	addForceFlag(cmd.Flags(), &force)
	return cmd
}
