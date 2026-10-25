package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSSHAgentCmd is devvm ssh-agent [on|devvm|off], and devvm's own
// agent's add and keys.
func newSSHAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-agent [on|devvm|off]",
		Short: "Show or change SSH agent forwarding for this folder's VM",
		Long: "Show whether this folder's VM forwards an SSH agent: off (the default), on (yours) or devvm " +
			"(devvm's own, which holds only the keys added to it with devvm ssh-agent add). devvm ssh-agent " +
			"on, devvm or off changes it from the VM's next start: devvm restart. While it's on and you're " +
			"connected, anything in the VM can use the agent's keys.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: sshagent.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error {
				if len(args) == 0 {
					return env.SSHAgent(cmd.Context(), ui.Stdout())
				}
				return env.SetSSHAgent(cmd.Context(), args[0])
			}, msg.SSHAgentFailed)
			return nil
		},
	}
	cmd.AddCommand(newSSHAgentAddCmd(), newSSHAgentKeysCmd())
	return cmd
}

func newSSHAgentAddCmd() *cobra.Command {
	var o sshagent.AddOptions
	cmd := &cobra.Command{
		Use:   "add KEY_FILE",
		Short: "Load a key into devvm's own SSH agent",
		Long: "Load a private key into devvm's own SSH agent, starting it first, and record it in " +
			"~/.config/devvm/agent-keys with its fingerprint and options, replacing what was recorded for it. " +
			"devvm loads it again when a VM needs it (after a reboot, or once its lifetime ran out), only " +
			"while the file holds the same key. A key in a folder a VM mounts is refused.",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error { return env.AgentAdd(cmd.Context(), args[0], o) }, msg.AgentKeysFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Confirm, "confirm", false, "ask before each use of the key, through an ssh-askpass program")
	f.StringVar(&o.Lifetime, "lifetime", "", "how long the agent keeps the key once loaded: a `DURATION` like 30m or 8h, or forever (default 12h)")
	return cmd
}

func newSSHAgentKeysCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "keys",
		Short: "List the keys in devvm's own SSH agent",
		Long: "List the keys devvm's own SSH agent holds, as ssh-add -l does, loading those it lacks first, " +
			"then each key added to it: its file, its fingerprint and how it's loaded.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(func(env *vm.Env) error { return env.AgentKeys(cmd.Context(), ui.Stdout()) }, msg.AgentKeysFailed)
			return nil
		},
	}
}
