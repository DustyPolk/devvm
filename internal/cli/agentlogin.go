package cli

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/agentlogin"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// agentLoginOpts are devvm agent login's flags.
type agentLoginOpts struct {
	device, apiKey, stdin bool
	provider              string
}

// newAgentLoginCmd is devvm agent login AGENT.
func newAgentLoginCmd(a *app) *cobra.Command {
	var o agentLoginOpts
	cmd := &cobra.Command{
		Use:   "login AGENT",
		Short: "Log an agent in, in this folder's VM, and save the login in devvm's store",
		Long: "Log AGENT, codex or opencode, in to ChatGPT inside this folder's VM, in a browser on this machine, or with a code with --device-code, " +
			"or save an API key with --api-key. " +
			"devvm's store keeps the login, so every VM with the agent gets it, and the VMs get only placeholders.",
		Example: examples(
			"devvm agent login codex", "log Codex in to ChatGPT in a browser on this machine",
			"devvm agent login codex --device-code", "log in with a code, in a browser on any machine",
			"devvm agent login opencode --api-key --provider anthropic", "save opencode's Anthropic API key, asking for it",
		),
		Args:      cobra.MatchAll(exactArgs(1), func(_ *cobra.Command, args []string) error { return o.check(args[0]) }),
		ValidArgs: vm.LoginAgents(),
		RunE: func(cmd *cobra.Command, args []string) error {
			lo := vm.AgentLoginOptions{Agent: args[0], Device: o.device, Provider: o.provider}
			if o.apiKey {
				key, err := o.askKey(a, lo)
				if err != nil {
					return err
				}
				if strings.TrimSpace(key) == "" {
					return msg.AgentKeyEmpty()
				}
				lo.Key = key
			}
			return a.env(func(env *vm.Env) error { return vm.AgentLogin{Env: env}.Run(cmd.Context(), lo) }, msg.AgentLoginFailed)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.device, "device-code", false, "log in with a code to type into a browser on any machine, so nothing is forwarded")
	f.BoolVar(&o.apiKey, "api-key", false, "save an API key instead, asking for it")
	f.StringVar(&o.provider, "provider", "", "with opencode's --api-key, the `PROVIDER` it's for: anthropic, openai or openrouter")
	f.BoolVar(&o.stdin, "stdin", false, "read the secret from a line of input, without asking")
	exclusive(cmd, "device-code", "api-key")
	addFolderFlags(cmd, a)
	return cmd
}

// check refuses flags that don't go with agent's login: --provider only
// with opencode's --api-key, which needs it, and --stdin only with
// --api-key.
func (o agentLoginOpts) check(agent string) error {
	providers := agentlogin.KeyProviders(agentlogin.Opencode)
	switch {
	case o.stdin && !o.apiKey:
		return msg.AgentLoginStdinAlone(agent)
	case o.provider != "" && (!o.apiKey || agent != agentlogin.Opencode):
		return msg.AgentLoginProviderAlone()
	case o.apiKey && agent == agentlogin.Opencode && o.provider == "":
		return msg.AgentLoginProviderNeeded(strings.Join(providers, ", "))
	case o.provider != "" && !slices.Contains(providers, o.provider):
		return msg.ArgNotChoice(strings.Join(providers, ", "), "--provider", o.provider)
	}
	return nil
}

// askKey asks for the API key without showing it, or with --stdin reads
// a line.
func (o agentLoginOpts) askKey(a *app, lo vm.AgentLoginOptions) (string, error) {
	if o.stdin {
		return a.u.Line(), nil
	}
	provider := lo.Provider
	if provider == "" {
		provider = "openai"
	}
	return a.u.Secret(msg.AgentKeyPrompt().AnsweredBy("--stdin").About(msg.AgentKeyHowto(lo.Agent, provider)))
}
