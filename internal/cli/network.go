package cli

import (
	"io"
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
	var global, asJSON bool
	cmd := &cobra.Command{
		Use:   "network [on|off]",
		Short: "Show or change the network policy of this folder's VM",
		Long: "Show the network policy of this folder's VM, which is on, off or unenforced, and then its allowlist, file by file. " +
			"devvm network on or off changes it from the VM's next start, such as with devvm restart. " +
			"With -g, show or change whether new VMs get network policy instead: ask, on or off.",
		Example: examples(
			"devvm network", "show the VM's network policy and allowlist",
			"devvm network allow example.com", "let the VM reach example.com",
			"devvm network log --blocked", "list what the VM's network policy blocked",
			"devvm network off", "turn network policy off from the VM's next start",
			"devvm network -g on", "give every new VM network policy without asking",
		),
		Args: showsOnly(func(cmd *cobra.Command, args []string) error {
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
		}),
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
			var (
				mode  string
				shown vm.NetworkShown
			)
			withEnv(u, func(env *vm.Env) error {
				n := vm.Net{Env: env}
				var err error
				switch {
				case global && len(args) == 0:
					mode, err = n.NetworkDefault()
				case global:
					err = n.SetNetworkDefault(args[0])
				case len(args) == 0:
					shown, err = n.Network(cmd.Context())
				default:
					err = n.SetNetwork(cmd.Context(), args[0] == state.NetworkOn)
				}
				return err
			}, failed)
			switch {
			case len(args) == 1:
				return nil
			case global:
				return printValue(u, asJSON, "policy", mode)
			}
			return printNetwork(u, asJSON, shown)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show or change whether new VMs get network policy instead")
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newNetworkRuleCmds(u)...)
	cmd.AddCommand(newNetworkLogCmd(u))
	return cmd
}

// networkOutput is devvm network --json: the VM's network policy and its
// allowlist, each policy file with rules in the order its proxy reads
// them.
type networkOutput struct {
	Policy    string           `json:"policy"`
	Allowlist []policyFileJSON `json:"allowlist"`
}

type policyFileJSON struct {
	File  string   `json:"file"`
	Rules []string `json:"rules"`
}

// printNetwork writes the VM's network policy: its line then its rules,
// or with asJSON as networkOutput.
func printNetwork(u *ui.UI, asJSON bool, s vm.NetworkShown) error {
	if !asJSON {
		if _, err := io.WriteString(u.Stdout(), s.Policy+"\n"); err != nil {
			return err
		}
		return netpolicy.WriteRules(u.Stdout(), s.Rules)
	}
	out := networkOutput{Policy: s.Policy, Allowlist: []policyFileJSON{}}
	for _, f := range s.Rules {
		out.Allowlist = append(out.Allowlist, policyFileJSON{File: f.Path, Rules: f.Rules})
	}
	return printJSON(u, out)
}

// newNetworkRuleCmds are devvm network allow, deny and rm RULE... [-g].
func newNetworkRuleCmds(u *ui.UI) []*cobra.Command {
	sub := func(use, short, long, example string, deny, remove bool) *cobra.Command {
		var global bool
		c := &cobra.Command{
			Use:     use + " RULE...",
			Short:   short,
			Long:    long,
			Example: example,
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
	rules := " RULE is a host, *.host for its subdomains, ** for any host, or an address or CIDR, " +
		"and with a port, such as example.com:8443, it matches only that port instead of 443 and 80."
	return []*cobra.Command{
		sub("allow", "Allow hosts for this folder's VM", "Add RULEs to the allowlist of this folder's VM, "+
			"or with -g of every VM, from the next connection on."+rules,
			examples(
				"devvm network allow example.com", "let the VM reach example.com",
				"devvm network allow '*.example.com'", "and any of its subdomains",
				"devvm network allow 10.0.0.5:5432", "one port of one address",
				"devvm network allow -g pypi.org", "let every VM reach pypi.org",
			), false, false),
		sub("deny", "Deny hosts for this folder's VM", "Add RULEs as denies to this folder's VM's rules, "+
			"or with -g every VM's, from the next connection on, and a deny beats any allow."+rules,
			examples(
				"devvm network deny example.com", "keep the VM from reaching example.com",
				"devvm network deny -g '*.example.com'", "keep every VM from its subdomains",
			), true, false),
		sub("rm", "Remove rules from this folder's VM", "Remove RULEs, as written with a deny's !, from "+
			"this folder's VM's own rules, or with -g yours for every VM. The change applies at once.",
			examples(
				"devvm network rm example.com", "remove an allow",
				"devvm network rm '!example.com'", "remove a deny",
			), false, true),
	}
}

// newNetworkLogCmd is devvm network log [--follow] [-n N] [--blocked].
func newNetworkLogCmd(u *ui.UI) *cobra.Command {
	var lines uint
	var o vm.LogOptions
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show what this folder's VM connected to and what its network policy blocked",
		Long: "Show the network log of this folder's VM, which it keeps only with network policy, with one line per connection " +
			"or request its proxy allowed or blocked, oldest first. " +
			"--blocked lists the blocks instead, one row per host and port, with the devvm network allow " +
			"command that allows each.",
		Example: examples(
			"devvm network log", "show the last 50 connections and requests",
			"devvm network log --follow", "keep showing new ones until Ctrl+C",
			"devvm network log --blocked", "list the blocks, with the command that allows each",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Lines = int(min(lines, 1<<31-1))
			if asJSON {
				return printLogJSON(cmd, u, o)
			}
			withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.Log(cmd.Context(), u.Stdout(), o) }, msg.LogFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.UintVarP(&lines, "lines", "n", 50, "show the last `N` connections and requests")
	f.BoolVar(&o.Follow, "follow", false, "keep showing new ones as they happen, until interrupted")
	f.BoolVar(&o.Blocked, "blocked", false, "list the blocks by host and port, with the command that allows each")
	jsonFlag(cmd, &asJSON)
	exclusive(cmd, "blocked", "follow")
	exclusive(cmd, "blocked", "lines")
	exclusive(cmd, "json", "follow")
	return cmd
}

// logEventJSON is an event in devvm network log --json: what the VM's
// proxy logged of a connection or request, leaving out what it lacks.
type logEventJSON struct {
	Time     string   `json:"time"`
	Event    string   `json:"event"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Method   string   `json:"method,omitempty"`
	Path     string   `json:"path,omitempty"`
	Rule     string   `json:"rule,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Agent    string   `json:"agent,omitempty"`
	Secrets  []string `json:"secrets,omitempty"`
	Refused  bool     `json:"refused,omitempty"`
	Upstream string   `json:"upstream,omitempty"`
}

// logOutput is devvm network log --json; blockedOutput, with --blocked.
type logOutput struct {
	Events []logEventJSON `json:"events"`
}

type blockJSON struct {
	Last  string `json:"last"`
	Count int    `json:"count"`
	Host  string `json:"host"`
	Port  int    `json:"port"`
	Allow string `json:"allow,omitempty"`
}

type blockedOutput struct {
	Blocked []blockJSON `json:"blocked"`
}

// printLogJSON writes the folder's VM's network log as logOutput, or with
// o.Blocked its blocks as blockedOutput.
func printLogJSON(cmd *cobra.Command, u *ui.UI, o vm.LogOptions) error {
	var (
		events []netpolicy.Event
		blocks []netpolicy.Block
	)
	withEnv(u, func(env *vm.Env) error {
		var err error
		if o.Blocked {
			blocks, err = vm.Net{Env: env}.LogBlocks(cmd.Context())
		} else {
			events, err = vm.Net{Env: env}.LogEvents(cmd.Context(), o.Lines)
		}
		return err
	}, msg.LogFailed)
	if o.Blocked {
		out := blockedOutput{Blocked: make([]blockJSON, 0, len(blocks))}
		for _, b := range blocks {
			out.Blocked = append(out.Blocked, blockJSON(b))
		}
		return printJSON(u, out)
	}
	out := logOutput{Events: make([]logEventJSON, 0, len(events))}
	for _, e := range events {
		out.Events = append(out.Events, logEventJSON{
			Time: e.Time, Event: e.Event, Host: e.Host, Port: e.Port, Method: e.Method, Path: e.Path, Rule: e.Rule,
			Reason: e.Reason, Agent: e.Agent, Secrets: e.Secrets, Refused: e.Refused, Upstream: e.Upstream,
		})
	}
	return printJSON(u, out)
}
