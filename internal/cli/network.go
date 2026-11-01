package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newNetworkCmd is devvm network [on|off], with -g [on|off|ask] for what
// new VMs get, and allow, deny, rm RULE... [-g] and log.
func newNetworkCmd(u *ui.UI) *cobra.Command {
	var global bool
	cmd := &cobra.Command{
		Use:   "network [on|off]",
		Short: "Show or change the network policy of this folder's VM",
		Long: "Show the network policy of this folder's VM, on, off or unenforced (on, but not confined: " +
			"started outside devvm, or before it was turned on), then its allowlist: each policy file that " +
			"has rules, in order (its image's, yours for every VM, the VM's own, its secrets' hosts), and " +
			"the rules in it. devvm network on or off changes the VM's network policy from its next start: " +
			"devvm restart. Secrets the VM got while it was off still work once it's on. With -g, show or " +
			"change whether new VMs get network policy instead: ask (in a terminal, and on without one), " +
			"on or off. devvm's --network-policy and --no-network-policy still choose for one new VM.",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := maxArgs(1)(cmd, args); err != nil || len(args) == 0 {
				return err
			}
			switch {
			case !global && args[0] == state.NetworkAsk:
				return msg.NetAskGlobal()
			case !slices.Contains(state.NetworkChoices(), args[0]):
				choices := []string{state.NetworkOn, state.NetworkOff}
				if global {
					choices = state.NetworkChoices()
				}
				return msg.ArgNotChoice(strings.Join(choices, ", "), cmd.CommandPath(), msg.Shown(args[0]))
			}
			return nil
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			switch {
			case len(args) > 0:
				return nil, cobra.ShellCompDirectiveNoFileComp
			case global:
				return state.NetworkChoices(), cobra.ShellCompDirectiveNoFileComp
			}
			return []cobra.Completion{state.NetworkOn, state.NetworkOff}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			failed := msg.NetworkFailed
			if global {
				failed = msg.NetDefaultFailed
			}
			withEnv(u, func(env *vm.Env) error {
				n := vm.Net{Env: env}
				switch {
				case global && len(args) == 0:
					return n.NetworkDefault(u.Stdout())
				case global:
					return n.SetNetworkDefault(args[0])
				case len(args) == 0:
					return n.Network(cmd.Context(), u.Stdout())
				}
				return n.SetNetwork(cmd.Context(), args[0] == state.NetworkOn)
			}, failed)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show or change whether new VMs get network policy instead")
	cmd.AddCommand(newNetworkRuleCmds(u)...)
	cmd.AddCommand(newNetworkLogCmd(u))
	return cmd
}

// newNetworkRuleCmds are devvm network allow, deny and rm RULE... [-g].
func newNetworkRuleCmds(u *ui.UI) []*cobra.Command {
	sub := func(use, short, long string, deny, remove bool) *cobra.Command {
		var global bool
		c := &cobra.Command{
			Use:   use + " RULE...",
			Short: short,
			Long:  long,
			Args: cobra.MatchAll(minArgs(1), func(_ *cobra.Command, args []string) error {
				if remove {
					return nil // as written, a deny with its !
				}
				for _, r := range args {
					if _, err := netpolicy.Rule(r, deny); err != nil {
						return err
					}
				}
				return nil
			}),
			RunE: func(cmd *cobra.Command, args []string) error {
				withEnv(u, func(env *vm.Env) error {
					return vm.Net{Env: env}.PolicyChange(cmd.Context(), args, deny, remove, global)
				}, msg.PolicyFailed)
				return nil
			},
		}
		c.Flags().BoolVarP(&global, "global", "g", false, "change your rules for every VM instead")
		return c
	}
	rules := " RULE is a host (example.com), any subdomain of one (*.example.com), any host (**), an " +
		"address or CIDR (10.0.0.5, fd00::/8), each with an optional port (example.com:8443, " +
		"[fd00::1]:22); without a port, 443 and 80."
	return []*cobra.Command{
		sub("allow", "Allow hosts for this folder's VM", "Add RULEs to the allowlist of this folder's VM, "+
			"or with -g of every VM. They apply at once, to the next connection."+rules, false, false),
		sub("deny", "Deny hosts for this folder's VM", "Add RULEs as denies to this folder's VM's rules, "+
			"or with -g every VM's: a deny beats any allow. They apply at once, to the next connection."+rules,
			true, false),
		sub("rm", "Remove rules from this folder's VM", "Remove RULEs, as written (a deny with its !), from "+
			"this folder's VM's own rules, or with -g yours for every VM. They apply at once.", false, true),
	}
}

// newNetworkLogCmd is devvm network log [-f] [-n N] [--blocked].
func newNetworkLogCmd(u *ui.UI) *cobra.Command {
	var lines uint
	var o vm.LogOptions
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show what this folder's VM connected to and what its network policy blocked",
		Long: "Show the network log of this folder's VM, which has network policy: one line per connection " +
			"or request its proxy allowed or blocked, oldest first, with the time, the host and port, and " +
			"the rule that allowed it, the request and the secrets put in, or why it was blocked. " +
			"--blocked lists the blocks instead, one row per host and port, with the devvm network allow " +
			"command that allows each.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Lines = int(min(lines, 1<<31-1))
			withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.Log(cmd.Context(), u.Stdout(), o) }, msg.LogFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.UintVarP(&lines, "lines", "n", 50, "show the last `N` connections and requests")
	f.BoolVarP(&o.Follow, "follow", "f", false, "then show each new one as it happens, until interrupted")
	f.BoolVar(&o.Blocked, "blocked", false, "list the blocks by host and port, with the command that allows each")
	exclusive(cmd, "blocked", "follow")
	exclusive(cmd, "blocked", "lines")
	return cmd
}
