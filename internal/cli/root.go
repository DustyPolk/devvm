// Package cli holds devvm's commands.
package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
)

// NewRoot returns the devvm command with every subcommand registered, each
// printing and asking through u. devvm itself is up: a shell in this
// folder's VM.
func NewRoot(u *ui.UI) *cobra.Command {
	g := new(globals)
	uf := new(upFlags)
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
			return up(cmd.Context(), u, g, uf, args)
		},
	}
	root.SetOut(u.Stdout())
	addGlobals(root, g)
	addUpFlags(root, uf)
	addVersion(root)
	root.AddCommand(newBuildCmd(u, g), newRebuildCmd(u, g), newImagesCmd(u, g), newGCCmd(u, g), newLayerCmd(u), newSetupCmd(u), newMsgCmd(u),
		newRestartCmd(u, g), newStopCmd(u), newRemoveCmd(u, g), newUpdateCmd(u, g), newAdoptCmd(u), newExportCmd(u), newRmCmd(u), newLsCmd(u), newLogCmd(u), newPolicyCmd(u), newNetworkCmd(u), newSSHCmd(u), newCodeCmd(u), newSSHAgentCmd(u), newGitProtectCmd(u), newPortsCmd(u), newSizeCmd(u), newDoctorCmd(u, g), newLimactlCmd(u), newQEMUCmd(u), newProxyCmd(u), newRefreshToolsCmd(u), newSecretCmd(u),
		newAuthCmd(u), newVersionCmd(u), newCompletionCmd(u))
	addCompletions(root, u)
	return root
}

// Execute runs devvm with args on u and returns its exit code. A command's
// error is printed like Warn: a message as it is, the command line
// parser's own ("unknown command") in usage_error.
func Execute(ctx context.Context, u *ui.UI, args []string) int {
	root := NewRoot(u)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		m, ok := errors.AsType[msg.M](err)
		if !ok {
			m = msg.UsageError(err.Error())
		}
		u.Warn(m)
		return 1
	}
	return 0
}
