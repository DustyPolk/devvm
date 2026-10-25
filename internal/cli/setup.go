package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// newSetupCmd is devvm setup [--features LIST] [--backend NAME]. Its
// --backend is its own, in place of the root's for new VMs.
func newSetupCmd() *cobra.Command {
	var features, backendName string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Choose base's features and the default backend",
		Long: "Choose the optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, and " +
			"your own in ~/.config/devvm/features. In a terminal it asks, starting from the current " +
			"choice; --features chooses them instead. A new choice makes base and the images built on it " +
			"rebuild when next used. --backend sets the backend new VMs get by default; with only it, " +
			"the features stay as they are.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var list *string
			if cmd.Flags().Changed("features") {
				list = &features
			}
			fail(setup(list, backendName), msg.SetupFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&features, "features", "", "the `LIST` of features base gets, without asking: fzf,omb, or '' for none")
	f.StringVar(&backendName, "backend", "", "the `NAME` of the backend new VMs get by default")
	return cmd
}

// setup saves the default backend when one is given, then base's features:
// features' (nil: ask, unless only the backend was given). A change to
// them says base rebuilds, and that devvm gc deletes the old images.
func setup(features *string, backendName string) error {
	dirs, err := paths.FromEnv()
	if err != nil {
		return err
	}
	if backendName != "" {
		reg, err := backends.Registry(dirs)
		if err != nil {
			return err
		}
		if _, ok := reg.Lookup(backendName); !ok {
			return msg.BackendUnknown(strings.Join(reg.Names(), ", "), backendName)
		}
		if err := state.WriteBackend(dirs, backendName); err != nil {
			return err
		}
		ui.Info(msg.DefaultBackend(backendName))
		if features == nil {
			return nil
		}
	}
	root, err := paths.Root()
	if err != nil {
		return err
	}
	b := &build.Builder{Dirs: dirs, Root: root}
	var changed bool
	if features == nil {
		changed, err = b.PickFeatures()
	} else {
		changed, err = b.SetFeatures(splitList(*features))
	}
	if err != nil || !changed {
		return err
	}
	ui.Info(msg.BaseWillRebuild())
	if built, err := state.AnyImage(dirs); err != nil {
		return err
	} else if built {
		ui.Say(msg.GCOldHint())
	}
	return nil
}

// splitList splits a comma-separated list, dropping empty items.
func splitList(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
