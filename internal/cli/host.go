package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newHostCmd is devvm host [--ssh-config].
func newHostCmd(u *ui.UI) *cobra.Command {
	var include bool
	cmd := &cobra.Command{
		Use:   "host",
		Short: "Get this folder's VM ready for ssh and editors, and show its host name",
		Long: "Get this folder's VM ready for ssh and editors as devvm does before opening a shell: checked, " +
			"started if it's stopped, its network policy checked, set up and in devvm's ssh config. Then show " +
			"the host name ssh and VS Code's Remote-SSH reach it by (devvm-website, for a folder named website). Without the Include line in " +
			"~/.ssh/config, devvm offers to add it, and --ssh-config adds it without asking.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error {
				env.AddInclude = include
				return vm.Launch{Env: env}.ShowHost(cmd.Context())
			}, msg.SSHFailed)
			return nil
		},
	}
	addSSHConfigFlag(cmd.Flags(), &include)
	return cmd
}
