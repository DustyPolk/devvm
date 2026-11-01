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
	var global, asJSON bool
	// run shows the ports without a change, else makes it.
	run := func(cmd *cobra.Command, c *vm.PortsChange) error {
		var ports string
		withEnv(u, func(env *vm.Env) error {
			s := vm.Settings{Env: env}
			if c != nil {
				c.Global = global
				return s.SetPorts(cmd.Context(), *c)
			}
			var err error
			ports, err = s.Ports(cmd.Context(), global)
			return err
		}, msg.PortsFailed)
		if c != nil {
			return nil
		}
		return printValue(u, asJSON, "ports", ports)
	}
	cmd := &cobra.Command{
		Use:   "ports [all|none|LIST]",
		Short: "Show or change which ports this folder's VM forwards",
		Long: "Show which ports this folder's VM forwards to the same port on this machine's localhost, " +
			"or change them at once to all, none or a LIST of ports and ranges like 5173,8000-8010. " +
			"With -g, show or change what new VMs forward instead.",
		Example: examples(
			"devvm ports", "show what the VM forwards",
			"devvm ports 5173,8000-8010", "forward only these ports and ranges",
			"devvm ports none", "forward nothing",
			"devvm ports -g none", "make new VMs forward nothing",
		),
		Args: showsOnly(cobra.MatchAll(maxArgs(1), func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			_, err := vm.ParsePorts(args[0])
			return err
		})),
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
			return run(cmd, c)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show or change what new VMs forward instead")
	jsonFlag(cmd, &asJSON)
	sub := func(op, short, long, example string) *cobra.Command {
		c := &cobra.Command{
			Use:     op + " LIST",
			Short:   short,
			Long:    long,
			Example: example,
			Args: cobra.MatchAll(exactArgs(1), func(_ *cobra.Command, args []string) error {
				_, err := vm.ParsePortList(args[0])
				return err
			}),
			RunE: func(cmd *cobra.Command, args []string) error {
				return run(cmd, &vm.PortsChange{Op: op, List: args[0]})
			},
		}
		c.Flags().BoolVarP(&global, "global", "g", false, "change what new VMs forward instead")
		return c
	}
	cmd.AddCommand(
		sub("add", "Forward more ports from this folder's VM", "Add LIST's ports and ranges, such as 5173,8000-8010, "+
			"to those this folder's VM forwards, or with -g to those new VMs forward. A VM that forwards "+
			"every port keeps doing so.",
			examples(
				"devvm ports add 5432", "forward one more port",
				"devvm ports add 8000-8010", "forward a range",
			)),
		sub("rm", "Stop forwarding some ports from this folder's VM", "Remove LIST's ports and ranges, each "+
			"as listed, from those this folder's VM forwards, or with -g from those new VMs forward. Removing "+
			"the last leaves none.",
			examples(
				"devvm ports rm 5173", "stop forwarding one port",
				"devvm ports rm -g 5173", "stop new VMs forwarding it",
			)),
	)
	return cmd
}
