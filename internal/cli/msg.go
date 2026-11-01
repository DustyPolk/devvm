package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// newMsgCmd is devvm _msg NAME key=value...: a message's text, for the
// end-to-end tests to look for. Hidden: it's not for users.
func newMsgCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:                "_msg NAME [key=value...]",
		Short:              "Print a message's text",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               minArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			m, err := msg.FromArgs(args[0], args[1:])
			if err != nil {
				return err
			}
			// The text whatever DEVVM_MESSAGES says: it's data here.
			_, err = fmt.Fprintln(u.Stdout(), m.Text())
			return err
		},
	}
}
