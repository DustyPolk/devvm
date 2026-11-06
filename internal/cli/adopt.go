package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newAdoptCmd is devvm adopt [VM] [--force].
func newAdoptCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "adopt [VM]",
		Short: "Give this folder the VM made for an earlier folder or path",
		Long: "Give this folder the VM made for an earlier folder at this path, or for this folder before you moved or renamed it, with its files, secrets and allowlist. " +
			"With VM, its name as devvm ls lists it, devvm moves that VM here from a folder that's gone, such as one you copied to another disk. " +
			"devvm asks first in a terminal without --no-input.",
		Example: examples(
			"devvm adopt", "give this folder the VM, after asking",
			"devvm adopt --force", "give it without asking, as a script must",
			"devvm adopt devvm-web-71a99b1076d92c", "move that VM here from a folder that's gone, after asking",
		),
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return a.env(func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Adopt(cmd.Context(), name, force) }, msg.AdoptFailed)
		},
	}
	addForceFlag(cmd.Flags(), &force)
	return cmd
}
