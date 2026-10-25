package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newNetworkCmd is devvm network [on|off].
func newNetworkCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "network [on|off]",
		Short: "Show or change the network policy of this folder's VM",
		Long: "Show the network policy of this folder's VM: on, off, or unenforced (on, but not confined: " +
			"started outside devvm, or before it was turned on). devvm network on or off changes it from " +
			"the VM's next start: devvm restart. Tokens the VM got while it was off still work once it's on.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error {
				if len(args) == 0 {
					return env.Network(cmd.Context(), u.Stdout())
				}
				return env.SetNetwork(cmd.Context(), args[0] == "on")
			}, msg.NetworkFailed)
			return nil
		},
	}
}
