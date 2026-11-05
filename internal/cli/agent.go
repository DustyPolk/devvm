package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newAgentCmd is devvm agent: devvm's store of the agents' settings and
// logins, and agent import, which fills it from this machine.
func newAgentCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Fill devvm's store of the agents' settings and logins that every VM gets",
		Long: "devvm keeps the coding agents' settings in ~/.config/devvm/agents and their logins in ~/.local/share/devvm/logins, " +
			"and every VM with an agent gets them from there, never from the agents' own files in your home folder. " +
			"devvm agent import copies those files into the store once.",
		Example: examples(
			"devvm agent import", "copy every agent's settings and logins from this machine",
			"devvm agent import codex", "copy only Codex's",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newAgentImportCmd(a))
	return cmd
}

// newAgentImportCmd is devvm agent import [AGENT...] [--force].
func newAgentImportCmd(a *app) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "import [AGENT...]",
		Short: "Copy the agents' settings and logins from this machine into devvm's store",
		Long: "Copy the settings and logins of AGENT, claude, codex or opencode, or of every agent, from your home folder into devvm's store, never their history. " +
			"It follows no link out of an agent's folder, and it asks before replacing what the store has.",
		Example: examples(
			"devvm agent import", "copy every agent's settings and logins, asking before replacing the store's",
			"devvm agent import claude codex", "copy only Claude Code's and Codex's",
			"devvm agent import --force", "replace the store's without asking, as a script must",
		),
		Args:      minArgs(0),
		ValidArgs: image.Agents,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := a.dirs()
			if err != nil {
				return failed(err, msg.AgentImportFailed)
			}
			store, err := a.secrets()
			if err != nil {
				return failed(err, msg.AgentImportFailed)
			}
			im := vm.AgentImport{Dirs: d, Secrets: store, Reporter: a.u}
			return failed(im.Run(cmd.Context(), args, force), msg.AgentImportFailed)
		},
	}
	addForceFlag(cmd.Flags(), &force)
	return cmd
}
