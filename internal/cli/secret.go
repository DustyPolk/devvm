package cli

import (
	"os"
	"strings"

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
	cmd.Args = exactArgs(1)
	cmd.ValidArgs = secretNames()
}

// newSecretCmd is devvm secret: the secrets kept on this machine, listed.
func newSecretCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "secret",
		Short: "List the secrets kept on this machine, for VMs to use",
		Long: "List the secrets devvm keeps on this machine, whether each is saved, and the VMs using it: " +
			"claude, the Claude Code token every VM whose image has claude uses, and github, the GitHub " +
			"token of the VMs it's bound to. With network policy a VM gets a placeholder, and its proxy " +
			"puts the secret into requests for the secret's hosts. Without network policy, the VM gets the " +
			"secret itself.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return listSecrets(cmd, u, asJSON) },
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newSecretSetCmd(u), newSecretRmCmd(u),
		newSecretUseCmd(u, "use", true), newSecretUseCmd(u, "unuse", false), newSecretGetCmd(u))
	return cmd
}

func newSecretSetCmd(u *ui.UI) *cobra.Command {
	var stdin bool
	cmd := &cobra.Command{
		Use:   "set NAME",
		Short: "Save the Claude Code token or GitHub token on this machine",
		Long: "Save a secret on this machine, claude (the Claude Code token that claude setup-token prints) " +
			"or github (a GitHub token), in the macOS Keychain, else in ~/.config/devvm/secrets/NAME, a file " +
			"only you can read. It asks for the token without showing it, and --stdin reads a line instead. " +
			"Proxies that are running put the new token in at once.",
		RunE: func(cmd *cobra.Command, args []string) error {
			saveSecret(cmd, u, args[0], stdin)
			return nil
		},
	}
	oneSecret(cmd)
	cmd.Flags().BoolVar(&stdin, "stdin", false, "read the token from a line of input, without asking")
	return cmd
}

// saveSecret asks for a secret's token (with stdin, reads a line) and
// saves it.
func saveSecret(cmd *cobra.Command, u *ui.UI, name string, stdin bool) {
	var given string
	if stdin {
		given = u.Line()
	} else {
		if u.StdinIsTerminal() {
			if name == "claude" {
				u.Note(msg.ClaudeTokenHowto())
			} else {
				u.Note(msg.GithubTokenHowto())
			}
		}
		given = u.Secret(msg.TokenPrompt())
	}
	withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.SecretSave(cmd.Context(), name, given) }, msg.SecretFailed)
}

func newSecretRmCmd(u *ui.UI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm NAME",
		Short: "Delete a saved token",
		Long:  "Delete a saved token. Proxies running stop putting it in at once; what a VM did with it stays done.",
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.SecretRm(cmd.Context(), args[0]) }, msg.SecretFailed)
			return nil
		},
	}
	oneSecret(cmd)
	return cmd
}

// secretOutput is devvm secret --json: each secret's name, whether it's
// saved and the VMs using it, never its value.
type secretOutput struct {
	Secrets []secretJSON `json:"secrets"`
}

type secretJSON struct {
	Name   string   `json:"name"`
	Saved  bool     `json:"saved"`
	UsedBy []string `json:"used_by"`
}

// listSecrets writes the secrets as a table, or with asJSON as
// secretOutput.
func listSecrets(cmd *cobra.Command, u *ui.UI, asJSON bool) error {
	var ss []vm.SecretListed
	withEnv(u, func(env *vm.Env) error {
		var err error
		ss, err = vm.Net{Env: env}.Secrets(cmd.Context())
		return err
	}, msg.SecretFailed)
	if asJSON {
		out := secretOutput{Secrets: make([]secretJSON, 0, len(ss))}
		for _, s := range ss {
			out.Secrets = append(out.Secrets, secretJSON{Name: s.Name, Saved: s.Saved, UsedBy: append([]string{}, s.UsedBy...)})
		}
		return printJSON(u, out)
	}
	tab := u.Table()
	tab.Header("NAME", "SAVED", "USED BY")
	for _, s := range ss {
		saved := "no"
		if s.Saved {
			saved = "yes"
		}
		tab.Row(cell(s.Name), cell(saved), cell(strings.Join(s.UsedBy, ", ")))
	}
	return tab.Flush()
}

// newSecretUseCmd is devvm secret use (use) or unuse.
func newSecretUseCmd(u *ui.UI, name string, use bool) *cobra.Command {
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
			withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.SecretUse(cmd.Context(), args[0], use) }, msg.SecretFailed)
			return nil
		},
	}
	oneSecret(cmd)
	return cmd
}

// newSecretGetCmd is devvm secret get, which VMs' proxies run for a value.
func newSecretGetCmd(u *ui.UI) *cobra.Command {
	var toTerminal bool
	get := &cobra.Command{
		Use:    "get NAME",
		Short:  "Print a secret's value, for a VM's network policy proxy",
		Args:   exactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Not to a terminal, so a value isn't shown by accident.
			if u.StdoutIsTerminal() && !toTerminal {
				u.Die(msg.SecretToTerminal())
			}
			dirs, err := paths.FromEnv()
			var v string
			if err == nil {
				v, err = secrets.NewStore(dirs, os.Getenv("HOME"), u.Warn).Get(cmd.Context(), args[0])
			}
			fail(u, err, func(reason string) msg.M { return msg.SecretReadFailed(reason, msg.Shown(args[0])) })
			_, err = u.Stdout().Write([]byte(v + "\n"))
			return err
		},
	}
	get.Flags().BoolVar(&toTerminal, "to-terminal", false, "print it even when the output is a terminal")
	return get
}
