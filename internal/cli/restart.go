package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newRestartCmd is devvm restart.
func newRestartCmd(g *globals) *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart this folder's VM and open a shell in it",
		Long: "Stop this folder's VM, then start it and open a shell in it as devvm does: what applies " +
			"only when it starts (agent folder mounts, size, ports) applies.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(func(env *vm.Env) error { return env.Restart(cmd.Context(), g.options()) }, msg.UpFailed)
			return nil
		},
	}
}
