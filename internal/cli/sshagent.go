package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSSHAgentCmd is devvm ssh-agent [on|devvm|off], and devvm's own
// agent's add and rm.
func newSSHAgentCmd(u *ui.UI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ssh-agent [on|devvm|off]",
		Short: "Show or change SSH agent forwarding for this folder's VM",
		Long: "Show whether this folder's VM forwards an SSH agent: off (the default), on (yours) or devvm " +
			"(devvm's own, which holds only the keys added to it with devvm ssh-agent add). Then, once a key " +
			"is added, it lists the keys devvm's own agent holds, as ssh-add -l does, loading those it lacks " +
			"first, and each key added to it: its file, its fingerprint and how it's loaded. devvm ssh-agent " +
			"on, devvm or off changes the VM's forwarding from its next start: devvm restart. While it's on " +
			"and you're connected, anything in the VM can use the agent's keys.",
		Args:      cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
		ValidArgs: sshagent.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error {
				s := vm.Settings{Env: env}
				if len(args) == 0 {
					return s.SSHAgent(cmd.Context(), u.Stdout())
				}
				return s.SetSSHAgent(cmd.Context(), args[0])
			}, msg.SSHAgentFailed)
			if len(args) == 0 {
				withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentKeys(cmd.Context(), u.Stdout()) }, msg.AgentKeysFailed)
			}
			return nil
		},
	}
	cmd.AddCommand(newSSHAgentAddCmd(u), newSSHAgentRmCmd(u))
	return cmd
}

func newSSHAgentAddCmd(u *ui.UI) *cobra.Command {
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
			withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentAdd(cmd.Context(), args[0], o) }, msg.AgentKeysFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Confirm, "confirm", false, "ask before each use of the key, through an ssh-askpass program")
	f.StringVar(&o.Lifetime, "lifetime", "", "how long the agent keeps the key once loaded: a `DURATION` like 30m or 8h, or forever (default 12h)")
	return cmd
}

func newSSHAgentRmCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "rm KEY_FILE",
		Short: "Remove a key from devvm's own SSH agent",
		Long: "Remove a key added with devvm ssh-agent add from devvm's own SSH agent, and from " +
			"~/.config/devvm/agent-keys, so devvm doesn't load it again. The key file stays as it is, and " +
			"needn't still exist. A key that wasn't added is refused: devvm ssh-agent lists those that were.",
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Settings{Env: env}.AgentRemove(cmd.Context(), args[0]) }, msg.AgentKeysFailed)
			return nil
		},
	}
}
