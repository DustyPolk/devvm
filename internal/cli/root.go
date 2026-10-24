// Package cli holds devvm's commands.
package cli

import "github.com/spf13/cobra"

// NewRoot returns the devvm command with every subcommand registered.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:          "devvm",
		Short:        "One isolated VM per project folder",
		SilenceUsage: true,
		Args:         cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newMsgCmd())
	return root
}

// Execute runs devvm with args and returns its exit code.
func Execute(args []string) int {
	root := NewRoot()
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return 1
	}
	return 0
}
