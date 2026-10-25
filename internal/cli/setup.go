package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// newSetupCmd is devvm setup [--backend NAME] [--features LIST]. Its
// --backend is its own, in place of the root's for new VMs.
func newSetupCmd() *cobra.Command {
	var features, backendName string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Choose the default backend and base's features",
		Long: "Choose the backend new VMs get by default (lima, or smolvm, experimental), then the " +
			"optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, and your own in " +
			"~/.config/devvm/features. In a terminal it asks for both, starting from the current choices. " +
			"--backend chooses the backend without asking, and with only it the features stay as they are; " +
			"--features chooses the features without asking, and with only it the backend stays as it is. " +
			"A new choice of features makes base and the images built on it rebuild when next used.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var list *string
			if cmd.Flags().Changed("features") {
				list = &features
			}
			fail(newSetup().run(list, backendName), msg.SetupFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&features, "features", "", "the `LIST` of features base gets, without asking: fzf,omb, or '' for none")
	f.StringVar(&backendName, "backend", "", "the `NAME` of the backend new VMs get by default")
	return cmd
}

// setup is devvm setup's run: whether it's in a terminal, and how it asks
// for the backend.
type setup struct {
	terminal func() bool
	pickOne  func(msg.M, []ui.Choice) int
}

// newSetup returns a setup on ui's terminal and picker.
func newSetup() setup {
	return setup{
		terminal: func() bool { return ui.StdinIsTerminal() && ui.StdoutIsTerminal() },
		pickOne:  ui.PickOne,
	}
}

// run saves the default backend, then base's features: features' (nil:
// ask, unless only the backend was given). With neither given, it asks for
// both in a terminal. A change to the features says base rebuilds, and
// that devvm gc deletes the old images.
func (s setup) run(features *string, backendName string) error {
	dirs, err := paths.FromEnv()
	if err != nil {
		return err
	}
	ask := backendName == "" && features == nil && s.terminal()
	if backendName != "" || ask {
		reg, err := backends.Registry(dirs)
		if err != nil {
			return err
		}
		if backendName == "" {
			if backendName, err = s.pickBackend(dirs, reg); err != nil {
				return err
			}
		} else if _, ok := reg.Lookup(backendName); !ok {
			return msg.BackendUnknown(strings.Join(reg.Names(), ", "), backendName)
		}
		if err := state.WriteBackend(dirs, backendName); err != nil {
			return err
		}
		ui.Info(msg.DefaultBackend(backendName))
		if features == nil && !ask {
			return nil
		}
	}
	root, err := paths.Root()
	if err != nil {
		return err
	}
	b := newBuilder(dirs, root, nil, backend.Review{}, nil, "")
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

// pickBackend asks which backend new VMs get by default, starting from
// the current default (backend.Default unless set, or set to one devvm
// doesn't have), the experimental ones marked.
func (s setup) pickBackend(dirs paths.Dirs, reg *backend.Registry) (string, error) {
	current, err := state.ReadBackend(dirs)
	if err != nil {
		return "", err
	}
	if _, ok := reg.Lookup(current); !ok {
		current = backend.Default
	}
	names := reg.Names()
	choices := make([]ui.Choice, len(names))
	for i, n := range names {
		choices[i] = ui.Choice{Name: n, On: n == current}
		if backends.Experimental(n) {
			choices[i].Desc = msg.BackendExperimental().String()
		}
	}
	return names[s.pickOne(msg.BackendHelp(), choices)], nil
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
