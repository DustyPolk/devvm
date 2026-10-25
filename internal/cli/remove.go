package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newRemoveCmd is devvm remove TOOLS.
func newRemoveCmd(u *ui.UI, g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "remove TOOLS",
		Short: "Take shared tools out of this folder's VM, then open a shell in it",
		Long: "Take shared tools out of this folder's VM (TOOLS joined with +: python+claude), then open " +
			"a shell in it. They're no longer mounted in it; what the VM installed itself stays. Your " +
			"layers are in the VM's disk, so they can't be taken out: devvm rm, and make the VM again.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Launch{Env: env}.Remove(cmd.Context(), g.options(), args[0]) }, msg.UpFailed)
			return nil
		},
	}
}
