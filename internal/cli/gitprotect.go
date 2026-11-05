package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newGitProtectCmd is devvm git-protect [on|strict|off].
func newGitProtectCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "git-protect [on|strict|off]",
		Short: "Show or change .git protection for this folder's VM",
		Long: "Show this folder's VM's .git protection, which locks the git hooks and config that run code on this machine, " +
			"as on, strict, off, or clone for a --clone VM. " +
			"devvm git-protect on, strict or off changes it at once on a stopped VM, and on a running one when it next stops, so in a terminal devvm offers to restart it now.",
		Example: examples(
			"devvm git-protect", "show the VM's .git protection",
			"devvm git-protect off", "unlock .git, once the VM is stopped",
			"devvm git-protect on", "lock .git again as it is then",
		),
		Args:      showsOnly(maxArgs(1)),
		ValidArgs: gitprotect.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			var mode string
			a.env(func(env *vm.Env) error {
				s := vm.Settings{Env: env}
				if len(args) == 1 {
					return s.SetGitProtect(cmd.Context(), args[0])
				}
				var err error
				mode, err = s.GitProtect(cmd.Context())
				return err
			}, msg.GitProtectFailed)
			if len(args) == 1 {
				return nil
			}
			return printValue(a.u, asJSON, "mode", mode)
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}
