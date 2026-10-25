package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newAdoptCmd is devvm adopt.
func newAdoptCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "adopt",
		Short: "Use the VM made for an earlier folder at this path here",
		Long: "A VM belongs to the folder it was made for: its path, and the folder itself. When that " +
			"folder was deleted and a new one made at the same path, devvm won't use the VM for it; " +
			"adopt gives it to this folder, with its files, tokens and allowlist, after asking.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Adopt(cmd.Context()) }, msg.AdoptFailed)
			return nil
		},
	}
}
