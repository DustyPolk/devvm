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
// devvm itself is up: a shell in this folder's VM.
func NewRoot() *cobra.Command {
	g := new(globals)
	u := new(upFlags)
	root := &cobra.Command{
		Use:   "devvm [IMAGE]",
		Short: "Open a shell in this folder's VM, creating it first",
		Long: "Open a shell in this folder's VM, at the same path, creating the VM first. A new VM " +
			"gets IMAGE (base, your layers and the shared tools joined with +: node+python+claude), " +
			"else the toolchains the folder's files call for, with any agents IMAGE names on top, built " +
			"if it isn't. Only the folder is mounted, read-write, with the shared tools read-only, and " +
			"its .git hooks and config are locked against changes from the VM (macOS); with --clone, " +
			"nothing is mounted but the shared tools, and the VM gets its own clone of the repo. A VM that exists " +
			"is started if it's stopped and opened as it is; an IMAGE that names what it lacks adds that " +
			"in place, after asking.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return up(cmd.Context(), g, u, args)
		},
	}
	root.SetOut(ui.Stdout())
	addGlobals(root, g)
	addUpFlags(root, u)
	addVersion(root)
	root.AddCommand(newBuildCmd(g), newRebuildCmd(g), newMsgCmd(),
		newRestartCmd(g), newRemoveCmd(g), newUpdateCmd(g), newAdoptCmd(), newRmCmd(), newLsCmd(), newLogCmd(), newPolicyCmd(), newNetworkCmd(), newDoctorCmd(g), newQEMUCmd(), newProxyCmd(), newRefreshToolsCmd(), newSecretCmd(),
		newAuthCmd(), newVersionCmd(), newCompletionCmd())
	addCompletions(root)
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
