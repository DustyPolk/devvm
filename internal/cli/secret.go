package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/ui"
)

// newSecretCmd is devvm secret: tokens kept on this machine for VMs with
// network policy. Hidden while it has only get, which VMs' proxies run for
// a value.
func newSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "secret",
		Short:  "Tokens kept on this machine for VMs with network policy",
		Hidden: true,
	}
	var force bool
	get := &cobra.Command{
		Use:   "get NAME",
		Short: "Print a secret's value, for a VM's network policy proxy",
		Args:  cobra.ExactArgs(1),
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
	cmd.AddCommand(get)
	return cmd
}
