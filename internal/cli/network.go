package cli

import (
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newNetworkCmd is devvm network [on|off], with -g [on|off|ask] for what
// new VMs get, and allow, deny, rm RULE... [-g] and log.
func newNetworkCmd(a *app) *cobra.Command {
	u := a.u
	var global, asJSON bool
	cmd := &cobra.Command{
		Use:   "network [on|off]",
		Short: "Show or change the network policy of this folder's VM",
		Long: "Show the network policy of this folder's VM, which is on, off or unenforced, and then its allowlist, policy by policy: devvm's built-in ones by name, then your files by path. " +
			"devvm network on or off changes it from the VM's next start, so in a terminal devvm offers to restart a running VM now. " +
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
			case !global && args[0] == settings.NetworkAsk:
				return msg.NetAskGlobal()
			case !slices.Contains(settings.NetworkChoices(), args[0]):
				choices := []string{settings.NetworkOn, settings.NetworkOff}
				if global {
					choices = settings.NetworkChoices()
				}
				return msg.ArgNotChoice(strings.Join(choices, ", "), cmd.CommandPath(), args[0])
			}
			return nil
		}),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			switch {
			case len(args) > 0:
				return nil, cobra.ShellCompDirectiveNoFileComp
			case global:
				return settings.NetworkChoices(), cobra.ShellCompDirectiveNoFileComp
			}
			return []cobra.Completion{settings.NetworkOn, settings.NetworkOff}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			what := msg.NetworkFailed
			if global {
				what = msg.NetDefaultFailed
			}
			var (
				mode  settings.Value
				label string
				shown vm.NetworkShown
			)
			err := a.env(func(env *vm.Env) error {
				n := vm.Net{Env: env}
				var err error
				switch {
				case global && len(args) == 0:
					mode, err = n.NetworkDefault()
					label = mode.Source.Label(env.Dirs)
				case global:
					err = n.SetNetworkDefault(args[0])
				case len(args) == 0:
					shown, err = n.Network(cmd.Context())
				default:
					err = n.SetNetwork(cmd.Context(), args[0] == settings.NetworkOn)
				}
				return err
			}, what)
			switch {
			case err != nil || len(args) == 1:
				return err
			case global:
				return printDefault(u, asJSON, "policy", mode, label)
			}
			return printNetwork(u, asJSON, shown)
		},
	}
	cmd.Flags().BoolVarP(&global, "global", "g", false, "show or change whether new VMs get network policy instead")
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newNetworkRuleCmds(a)...)
	cmd.AddCommand(newNetworkLogCmd(a))
	return addFolderFlags(cmd, a)
}

// networkOutput is devvm network --json: the VM's network policy and its
// allowlist, each policy with rules in the order its proxy reads them.
type networkOutput struct {
	Policy    string           `json:"policy"`
	Allowlist []policyFileJSON `json:"allowlist"`
}

// policyFileJSON is a policy: a built-in one, by its name in devvm's
// files, or one of the user's files, by its path; its rules, and its
// lines that aren't rules, which its proxy skips.
type policyFileJSON struct {
	Builtin  string        `json:"builtin,omitempty"`
	File     string        `json:"file,omitempty"`
	Rules    []string      `json:"rules"`
	NotRules []notRuleJSON `json:"not_rules"`
}

// notRuleJSON is a policy file's line that isn't a rule: its number, from
// 1, and its text, comment left out.
type notRuleJSON struct {
	Line int    `json:"line"`
	Text string `json:"text"`
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
		j := policyFileJSON{Rules: append([]string{}, f.Rules()...), NotRules: []notRuleJSON{}}
		if f.Builtin {
			j.Builtin = f.Path
		} else {
			j.File = f.Path
		}
		for _, l := range f.NotRules() {
			j.NotRules = append(j.NotRules, notRuleJSON{Line: l.N, Text: l.Text})
		}
		out.Allowlist = append(out.Allowlist, j)
	}
	return printJSON(u, out)
}

// newNetworkRuleCmds are devvm network allow, deny and rm RULE... [-g].
func newNetworkRuleCmds(a *app) []*cobra.Command {
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
				return a.env(func(env *vm.Env) error {
					return vm.Net{Env: env}.PolicyChange(cmd.Context(), args, deny, remove, global)
				}, msg.PolicyFailed)
			},
		}
		c.Flags().BoolVarP(&global, "global", "g", false, "change your rules for every VM instead")
		return addFolderFlags(c, a)
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
func newNetworkLogCmd(a *app) *cobra.Command {
	var lines uint
	var o vm.LogOptions
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show what this folder's VM connected to and what its network policy blocked",
		Long: "Show the network log of this folder's VM, oldest first: with network policy, one line per connection " +
			"or request its proxy allowed or blocked, and with or without it, one line per connection to one of its links " +
			"and each that devvm refused. " +
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
				return printLogJSON(cmd, a, o)
			}
			return a.env(func(env *vm.Env) error { return vm.Net{Env: env}.Log(cmd.Context(), a.u.Stdout(), o) }, msg.LogFailed)
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
	return addFolderFlags(cmd, a)
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
	Target   string   `json:"target,omitempty"`
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
func printLogJSON(cmd *cobra.Command, a *app, o vm.LogOptions) error {
	u := a.u
	var (
		events []netpolicy.Event
		blocks []netpolicy.Block
	)
	if err := a.env(func(env *vm.Env) error {
		var err error
		if o.Blocked {
			blocks, err = vm.Net{Env: env}.LogBlocks(cmd.Context())
		} else {
			events, err = vm.Net{Env: env}.LogEvents(cmd.Context(), o.Lines)
		}
		return err
	}, msg.LogFailed); err != nil {
		return err
	}
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
			Reason: e.Reason, Agent: e.Agent, Secrets: e.Secrets, Refused: e.Refused, Upstream: e.Upstream, Target: e.Target,
		})
	}
	return printJSON(u, out)
}
