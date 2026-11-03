package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSSHAgentCmd is devvm ssh-agent [on|devvm|off], and devvm's own
// agent's add and rm.
func newSSHAgentCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ssh-agent [on|devvm|off]",
		Short: "Show or change SSH agent forwarding for this folder's VM",
		Long: "Show whether this folder's VM forwards an SSH agent, which is off, on for yours, or devvm for devvm's own " +
			"with only the keys you added to it, and then those keys. " +
			"devvm ssh-agent on, devvm or off changes it from the VM's next start, so in a terminal devvm offers to restart a running VM now. " +
			"While forwarding is on and you're connected, anything in the VM can use the agent's keys.",
		Example: examples(
			"devvm ssh-agent", "show the VM's agent forwarding",
			"devvm ssh-agent add ~/.ssh/deploy_key", "load a key into devvm's own agent",
			"devvm ssh-agent devvm", "forward devvm's own agent from the VM's next start",
			"devvm ssh-agent off", "stop forwarding an agent",
		),
		Args:      showsOnly(maxArgs(1)),
		ValidArgs: sshagent.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.SetSSHAgent(cmd.Context(), args[0]) }, msg.SSHAgentFailed)
				return nil
			}
			var mode string
			withEnv(u, func(env *vm.Env) error {
				var err error
				mode, err = vm.Settings{Env: env}.SSHAgent(cmd.Context())
				return err
			}, msg.SSHAgentFailed)
			if asJSON {
				return printAgentJSON(cmd, u, mode)
			}
			if _, err := io.WriteString(u.Stdout(), mode+"\n"); err != nil {
				return err
			}
			withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentKeys(cmd.Context(), u.Stdout()) }, msg.AgentKeysFailed)
			return nil
		},
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newSSHAgentAddCmd(u), newSSHAgentRmCmd(u))
	return cmd
}

// sshAgentOutput is devvm ssh-agent --json: the VM's forwarding, and the
// keys added to devvm's own agent.
type sshAgentOutput struct {
	Mode string         `json:"mode"`
	Keys []agentKeyJSON `json:"keys"`
}

type agentKeyJSON struct {
	File        string `json:"file"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Confirm     bool   `json:"confirm"`
	Lifetime    string `json:"lifetime"`
	Loaded      bool   `json:"loaded"`
}

// printAgentJSON writes the VM's forwarding mode and devvm's own agent's
// keys as sshAgentOutput.
func printAgentJSON(cmd *cobra.Command, u *ui.UI, mode string) error {
	var keys []sshagent.ListedKey
	withEnv(u, func(env *vm.Env) error {
		var err error
		keys, err = vm.Settings{Env: env}.AgentKeyList(cmd.Context())
		return err
	}, msg.AgentKeysFailed)
	out := sshAgentOutput{Mode: mode, Keys: make([]agentKeyJSON, 0, len(keys))}
	for _, k := range keys {
		out.Keys = append(out.Keys, agentKeyJSON(k))
	}
	return printJSON(u, out)
}

func newSSHAgentAddCmd(u *ui.UI) *cobra.Command {
	var o sshagent.AddOptions
	cmd := &cobra.Command{
		Use:   "add KEY_FILE",
		Short: "Load a key into devvm's own SSH agent",
		Long: "Load a private key into devvm's own SSH agent, starting the agent first, and record it in " +
			"~/.local/share/devvm/agent-keys.json, so devvm loads it again when a VM needs it. " +
			"devvm refuses a key in a folder a VM mounts.",
		Example: examples(
			"devvm ssh-agent add ~/.ssh/deploy_key", "load a key for 12 hours at a time",
			"devvm ssh-agent add ~/.ssh/deploy_key --lifetime 30m", "for 30 minutes at a time",
			"devvm ssh-agent add ~/.ssh/deploy_key --confirm", "ask before each use of the key",
		),
		Args: exactArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentAdd(cmd.Context(), args[0], o) }, msg.AgentKeysFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Confirm, "confirm", false, "ask before each use of the key, through an ssh-askpass program")
	f.StringVar(&o.Lifetime, "lifetime", "", "how long the agent keeps the key once loaded, a `DURATION` such as 8h, or forever (default 12h)")
	return cmd
}

func newSSHAgentRmCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "rm KEY_FILE",
		Short: "Remove a key from devvm's own SSH agent",
		Long: "Remove a key added with devvm ssh-agent add from devvm's own SSH agent, and from " +
			"~/.local/share/devvm/agent-keys.json, so devvm doesn't load it again. " +
			"The key file stays as it is, and it needn't still exist.",
		Example: examples(
			"devvm ssh-agent rm ~/.ssh/deploy_key", "take the key out of devvm's own agent",
		),
		Args: exactArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentRemove(cmd.Context(), args[0]) }, msg.AgentKeysFailed)
			return nil
		},
	}
}
