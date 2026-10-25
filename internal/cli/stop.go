package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newStopCmd is devvm stop.
func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop this folder's VM",
		Long: "Stop this folder's VM and its network policy proxy. Once it's stopped, its .git is locked " +
			"again and checked, as on a launch that finds it stopped, and its ssh config entry goes.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(func(env *vm.Env) error { return env.Stop(cmd.Context()) }, msg.StopVMFailed)
			return nil
		},
	}
}
