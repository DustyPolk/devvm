package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// secretNames are the secrets devvm keeps, for completion and checks.
func secretNames() []string {
	names := make([]string, len(secrets.Known))
	for i, s := range secrets.Known {
		names[i] = s.Name
	}
	return names
}

// oneSecret takes one argument: a secret devvm keeps.
func oneSecret(cmd *cobra.Command) {
	cmd.Args = cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs)
	cmd.ValidArgs = secretNames()
}

// newSecretCmd is devvm secret: tokens kept on this machine.
func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "Tokens kept on this machine, for VMs to use",
		Long: "Tokens kept on this machine: claude (every VM whose image has claude uses it) and github " +
			"(VMs it's bound to). With network policy a VM gets a placeholder and its proxy puts the token " +
			"into requests for the token's hosts; without it, the VM gets the token. devvm secret alone " +
			"lists them, as devvm secret ls does.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return listSecrets(cmd) },
	}
	cmd.AddCommand(newSecretSetCmd(), newSecretRmCmd(), newSecretLsCmd(),
		newSecretUseCmd("use", true), newSecretUseCmd("unuse", false), newSecretGetCmd())
	return cmd
}

func newSecretSetCmd() *cobra.Command {
	var stdin bool
	cmd := &cobra.Command{
		Use:   "set NAME",
		Short: "Save a token on this machine",
		Long: "Save a token on this machine, claude or github: in the macOS Keychain, else in " +
			"~/.config/devvm/secrets/NAME, a file only you can read. It asks for the token without showing " +
			"it; --stdin reads a line instead. A claude token comes from claude setup-token. Proxies running " +
			"put the new token in at once.",
		RunE: func(cmd *cobra.Command, args []string) error {
			saveSecret(cmd, args[0], stdin)
			return nil
		},
	}
	oneSecret(cmd)
	cmd.Flags().BoolVar(&stdin, "stdin", false, "read the token from a line of input, without asking")
	return cmd
}

// saveSecret asks for a secret's token (with stdin, reads a line) and
// saves it.
func saveSecret(cmd *cobra.Command, name string, stdin bool) {
	var given string
	if stdin {
		given = ui.Line()
	} else {
		if ui.StdinIsTerminal() {
			if name == "claude" {
				ui.Note(msg.ClaudeTokenHowto())
			} else {
				ui.Note(msg.GithubTokenHowto())
			}
		}
		given = ui.Secret(msg.TokenPrompt())
	}
	withEnv(func(env *vm.Env) error { return env.SecretSave(cmd.Context(), name, given) }, msg.SecretFailed)
}

func newSecretRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm NAME",
		Short: "Delete a saved token",
		Long:  "Delete a saved token. Proxies running stop putting it in at once; what a VM did with it stays done.",
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error { return env.SecretRm(cmd.Context(), args[0]) }, msg.SecretFailed)
			return nil
		},
	}
	oneSecret(cmd)
	return cmd
}

func newSecretLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List the tokens, whether each is saved, and the VMs using it",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return listSecrets(cmd) },
	}
}

func listSecrets(cmd *cobra.Command) error {
	withEnv(func(env *vm.Env) error { return env.Secrets(cmd.Context(), ui.Stdout()) }, msg.SecretFailed)
	return nil
}

// newSecretUseCmd is devvm secret use (use) or unuse.
func newSecretUseCmd(name string, use bool) *cobra.Command {
	short, long := "Let this folder's VM use a token", "Let this folder's VM use a token: "+
		"its proxy puts it in at once, and the VM gets its placeholder (the token, without network policy) "+
		"from its next launch."
	if !use {
		short, long = "Stop this folder's VM using a token", "Stop this folder's VM using a token: "+
			"its proxy stops putting it in at once. Without network policy the VM got the token itself, "+
			"which still works wherever it kept it. A VM whose image has claude always uses claude."
	}
	cmd := &cobra.Command{
		Use:   name + " NAME",
		Short: short,
		Long:  long,
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error { return env.SecretUse(cmd.Context(), args[0], use) }, msg.SecretFailed)
			return nil
		},
	}
	oneSecret(cmd)
	return cmd
}

// newSecretGetCmd is devvm secret get, which VMs' proxies run for a value.
func newSecretGetCmd() *cobra.Command {
	var force bool
	get := &cobra.Command{
		Use:    "get NAME",
		Short:  "Print a secret's value, for a VM's network policy proxy",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Not to a terminal, so a value isn't shown by accident.
			if ui.StdoutIsTerminal() && !force {
				ui.Die(msg.SecretToTerminal())
			}
			dirs, err := paths.FromEnv()
			var v string
			if err == nil {
				v, err = secrets.NewStore(dirs, os.Getenv("HOME")).Get(cmd.Context(), args[0])
			}
			fail(err, func(reason string) msg.M { return msg.SecretReadFailed(reason, msg.Shown(args[0])) })
			_, err = ui.Stdout().Write([]byte(v + "\n"))
			return err
		},
	}
	get.Flags().BoolVar(&force, "force", false, "print it to a terminal too")
	return get
}

// newAuthCmd is devvm auth claude: devvm secret set claude.
func newAuthCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "auth claude",
		Short:     "Save the Claude token: devvm secret set claude",
		Long:      "Save the Claude token that claude setup-token prints, as devvm secret set claude does.",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			saveSecret(cmd, "claude", false)
			return nil
		},
	}
}
