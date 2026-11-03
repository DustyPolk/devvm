// Package cli holds devvm's commands.
package cli

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// NewRoot returns the devvm command with every subcommand registered, each
// printing and asking through u. devvm itself is up: a shell in this
// folder's VM.
func NewRoot(u *ui.UI) *cobra.Command {
	uf := new(upFlags)
	fl := new(ui.Flags)
	root := &cobra.Command{
		Use:   "devvm",
		Short: "Open a shell in this folder's VM, creating it first",
		Long: "Open a shell in this folder's VM, at the same path, and create the VM first when the folder has none. " +
			"In a terminal you choose a new VM's toolchains, coding agents and layers in a list, and --tools, --agents and --layers choose them without it. " +
			"The VM mounts only this folder read-write, and with --clone it gets its own clone of the repo instead.",
		Example: examples(
			"devvm", "open this folder's VM, choosing what a new one gets",
			"devvm --agents claude", "create the VM with Claude Code and the toolchains the folder needs",
			"devvm --tools node --agents none --layers docker", "create the VM with node and the docker layer alone",
			"devvm --clone", "create the VM with its own clone of this repo",
			"devvm ls", "list every VM",
		),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			u.SetFlags(*fl)
			if wantsJSON(cmd) {
				u.DataOnly() // stdout is the JSON's alone
			}
			makePrivate(u)
			if !skipsRetention(cmd) {
				autoRetain(cmd.Context(), u)
			}
		},
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return up(cmd.Context(), u, uf)
		},
		SuggestionsMinimumDistance: msg.SuggestDistance,
	}
	root.SetOut(u.Stdout())
	root.PersistentFlags().BoolVarP(&fl.Verbose, "verbose", "v", false, "show the output of the commands devvm runs instead of a status line")
	root.PersistentFlags().BoolVar(&fl.NoInput, "no-input", false, "never ask, and fail where devvm would ask, naming the flag that answers it")
	addUpFlags(root, uf)
	addVersion(root)
	root.AddCommand(newBuildCmd(u), newRebuildCmd(u), newImagesCmd(u), newGCCmd(u), newSpaceCmd(u), newLayerCmd(u), newSetupCmd(u), newMsgCmd(u),
		newRestartCmd(u), newStopCmd(u), newToolsCmd(u), newAdoptCmd(u), newHistoryCmd(u), newRmCmd(u), newLsCmd(u), newNetworkCmd(u), newHostCmd(u), newCodeCmd(u), newDesktopCmd(u), newSSHAgentCmd(u), newGitProtectCmd(u), newPortsCmd(u), newSizeCmd(u), newDoctorCmd(u), newLimactlCmd(u), newQEMUCmd(u), newProxyCmd(u), newLimaPortsCmd(u), newRefreshToolsCmd(u), newSecretCmd(u),
		newLinkCmd(u), newGroupCmd(u), newLinksCmd(u), newRetentionCmd(u), newVersionCmd(u), newCompletionCmd(u))
	addCompletions(root)
	checkCommands(root)
	addHelp(root)
	addGroups(root)
	addDocs(root)
	return root
}

// makePrivate makes devvm's data and settings folders the user's alone on
// every run, whatever mode they had. Without a HOME there are no folders,
// and the command says so itself.
func makePrivate(u *ui.UI) {
	d, err := paths.FromEnv()
	if err != nil {
		return
	}
	if err := state.MakePrivate(d); err != nil {
		u.Warn(msg.FoldersNotPrivate(d.Config, d.Data).Because(err))
	}
}

// Execute runs devvm with args on u and returns its exit code: 0, 1 for a
// failure, usageCode for a usage error (a bad argument or flag, an
// unknown command), which is followed by usage_help, naming the
// command's own --help.
func Execute(ctx context.Context, u *ui.UI, args []string) int {
	return execute(ctx, u, NewRoot(u), args)
}

// execute runs root with args, as Execute does.
func execute(ctx context.Context, u *ui.UI, root *cobra.Command, args []string) (code int) {
	defer func() {
		if p := recover(); p != nil {
			w, ok := p.(wordPanic)
			if !ok {
				panic(p)
			}
			printUsage(u, root, w.err)
			code = usageCode
		}
	}()
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return 0
	}
	if cmd == nil {
		cmd = root
	}
	if e, ok := errors.AsType[ranError](err); ok {
		m, ok := errors.AsType[msg.M](e.err)
		if !ok {
			m = msg.OutputFailed(e.err.Error()).Because(e.err)
		}
		u.Error(m)
		return 1
	}
	// Anything else is the parser's, before the command ran. A word that
	// isn't a command is devvm's, whichever command (help) refused it.
	e := usage(err)
	if strings.HasPrefix(e.m.ID(), "no_command") {
		cmd = root
	}
	printUsage(u, cmd, e)
	return usageCode
}
