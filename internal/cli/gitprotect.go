package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newGitProtectCmd is devvm git-protect [on|strict|off].
func newGitProtectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "git-protect [on|strict|off]",
		Short: "Show or change .git protection for this folder's VM",
		Long: "Show whether this folder's VM can change the parts of .git that run code on this machine: " +
			"on (the default on macOS: hooks and config locked), strict (on, and host git status and diff " +
			"stay out of submodules), off, or clone for a --clone VM. devvm git-protect on, strict or off " +
			"changes it while the VM is stopped; on and strict lock .git as it is now. Locks need macOS.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: gitprotect.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error {
				if len(args) == 0 {
					return env.GitProtect(cmd.Context(), ui.Stdout())
				}
				return env.SetGitProtect(cmd.Context(), args[0])
			}, msg.GitProtectFailed)
			return nil
		},
	}
}
