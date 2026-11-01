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
	var verbose bool
	root := &cobra.Command{
		Use:   "devvm [IMAGE]",
		Short: "Open a shell in this folder's VM, creating it first",
		Long: "Open a shell in this folder's VM, at the same path, creating the VM first. A new VM " +
			"gets IMAGE (base, layers such as docker or your own, and the shared tools joined with +: " +
			"node+python+docker+claude), " +
			"else the toolchains (and the nix layer) the folder's files call for, with any agents IMAGE names on top, built " +
			"if it isn't. Only the folder is mounted, read-write, with the shared tools read-only, and " +
			"its .git hooks and config are locked against changes from the VM (macOS); with --clone, " +
			"nothing is mounted but the shared tools, and the VM gets its own clone of the repo. A new VM also gets " +
			"what the folder's devcontainer.json says it can, unless --no-devcontainer. A VM that exists " +
			"is started if it's stopped and opened as it is; an IMAGE that names what it lacks is refused, " +
			"naming the devvm tools add command that adds its shared tools, as only a new VM gets a layer. In a folder without a VM of its own, inside one that has a VM, devvm " +
			"asks whether to open that VM in this folder instead of creating one, and opens it without " +
			"asking when there's no terminal. --here and --clone create the new VM without asking.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			u.SetVerbose(verbose)
			if wantsJSON(cmd) {
				u.DataOnly() // stdout is the JSON's alone
			}
			makePrivate(u)
			if !skipsRetention(cmd) {
				autoRetain(cmd.Context(), u)
			}
		},
		Args: cobra.MatchAll(maxArgs(1), func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return checkWord(cmd, args[0])
		}),
		RunE: func(cmd *cobra.Command, args []string) error {
			return up(cmd.Context(), u, uf, args)
		},
		SuggestionsMinimumDistance: suggestDistance,
	}
	root.SetOut(u.Stdout())
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "show the output of the commands devvm runs as they run, as the run log gets it, instead of a status line")
	addUpFlags(root, uf)
	addVersion(root)
	root.AddCommand(newBuildCmd(u), newRebuildCmd(u), newImagesCmd(u), newGCCmd(u), newSpaceCmd(u), newLayerCmd(u), newSetupCmd(u), newMsgCmd(u),
		newRestartCmd(u), newStopCmd(u), newToolsCmd(u), newAdoptCmd(u), newHistoryCmd(u), newRmCmd(u), newLsCmd(u), newNetworkCmd(u), newHostCmd(u), newCodeCmd(u), newDesktopCmd(u), newSSHAgentCmd(u), newGitProtectCmd(u), newPortsCmd(u), newSizeCmd(u), newDoctorCmd(u), newLimactlCmd(u), newQEMUCmd(u), newProxyCmd(u), newLimaPortsCmd(u), newSmolvmPortsCmd(u), newDockerPortsCmd(u), newDockerConnectCmd(u), newRefreshToolsCmd(u), newSecretCmd(u),
		newRetentionCmd(u), newVersionCmd(u), newCompletionCmd(u))
	addCompletions(root, u)
	checkCommands(root)
	addHelp(root)
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
	// Anything else is the parser's, before the command ran. A word that's
	// neither a command nor an image is devvm's, whichever command (help)
	// refused it.
	e := usage(err)
	if strings.HasPrefix(e.m.ID(), "no_command_or_image") {
		cmd = root
	}
	printUsage(u, cmd, e)
	return usageCode
}
