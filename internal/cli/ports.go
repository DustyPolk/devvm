package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newPortsCmd is devvm ports [all|none|LIST], and add and rm LIST, each
// with -g for what new VMs forward.
func newPortsCmd(u *ui.UI) *cobra.Command {
	var global bool
	// run shows the ports without a change, else makes it.
	run := func(cmd *cobra.Command, c *vm.PortsChange) {
		withEnv(u, func(env *vm.Env) error {
			if c == nil {
				return env.Ports(cmd.Context(), u.Stdout(), global)
			}
			c.Global = global
			return env.SetPorts(cmd.Context(), *c)
		}, msg.PortsFailed)
	}
	cmd := &cobra.Command{
		Use:   "ports [all|none|LIST]",
		Short: "Show or change which ports this folder's VM forwards",
		Long: "Show which ports this folder's VM forwards to the same port on this machine's localhost: " +
			"all, none, or a LIST of ports and ranges like 5173,8000-8010. devvm ports all, none or LIST " +
			"changes it, at once for a stopped VM, from its next start for a running one: devvm restart. " +
			"With -g, show or change what new VMs forward instead.",
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return []cobra.Completion{vm.PortsAll, vm.PortsNone}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var c *vm.PortsChange
			if len(args) == 1 {
				c = &vm.PortsChange{List: args[0]}
			}
			run(cmd, c)
			return nil
		},
	}
	cmd.PersistentFlags().BoolVarP(&global, "global", "g", false, "show or change what new VMs forward instead")
	sub := func(op, short, long string) *cobra.Command {
		return &cobra.Command{
			Use:   op + " LIST",
			Short: short,
			Long:  long,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				run(cmd, &vm.PortsChange{Op: op, List: args[0]})
				return nil
			},
		}
	}
	cmd.AddCommand(
		sub("add", "Forward more ports from this folder's VM", "Add LIST's ports and ranges (5173,8000-8010) "+
			"to those this folder's VM forwards, or with -g to those new VMs forward. A VM that forwards "+
			"every port keeps doing so."),
		sub("rm", "Stop forwarding some ports from this folder's VM", "Remove LIST's ports and ranges, each "+
			"as listed, from those this folder's VM forwards, or with -g from those new VMs forward. Removing "+
			"the last leaves none."),
	)
	return cmd
}
