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
		Long: "A VM belongs to the folder it was made for: its path, and the folder itself. When that " +
			"folder was deleted and a new one made at the same path, devvm won't use the VM for it; " +
			"adopt gives it to this folder, with its files, tokens and allowlist, after asking in a " +
			"terminal. --force gives it without asking, which it needs without a terminal.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Adopt(cmd.Context(), force) }, msg.AdoptFailed)
			return nil
		},
	}
	addForceFlag(cmd.Flags(), &force)
	return cmd
}
