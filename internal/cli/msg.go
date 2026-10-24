package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
)

// newMsgCmd is devvm _msg NAME key=value...: a message's text, for the
// end-to-end tests to look for. Hidden: it's not for users.
func newMsgCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "_msg NAME [key=value...]",
		Short:              "Print a message's text",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := msg.FromArgs(args[0], args[1:])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), m.Text())
			return err
		},
	}
}
