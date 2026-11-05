package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newStopCmd is devvm stop.
func newStopCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop this folder's VM",
		Long: "Stop this folder's VM and its network policy proxy. " +
			"devvm, devvm restart or a command that needs the VM running starts it again.",
		Example: examples(
			"devvm stop", "stop this folder's VM",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a.env(func(env *vm.Env) error { return vm.Lifecycle{Env: env}.Stop(cmd.Context()) }, msg.StopVMFailed)
			return nil
		},
	}
	addFolderFlags(cmd, a)
	return cmd
}
