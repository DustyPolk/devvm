// Package cli holds devvm's commands.
package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// NewRoot returns the devvm command with every subcommand registered.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "devvm",
		Short:         "One isolated VM per project folder",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetOut(ui.Stdout())
	root.AddCommand(newBuildCmd(), newRebuildCmd(), newMsgCmd(), newQEMUCmd())
	return root
}

// Execute runs devvm with args and returns its exit code. A command's error
// is printed like ui.Warn: a message as it is, the command line parser's own
// ("unknown command") in usage_error.
func Execute(ctx context.Context, args []string) int {
	root := NewRoot()
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		m, ok := errors.AsType[msg.M](err)
		if !ok {
			m = msg.UsageError(err.Error())
		}
		ui.Warn(m)
		return 1
	}
	return 0
}
