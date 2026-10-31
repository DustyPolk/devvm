package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSetupCmd is devvm setup [--backend NAME] [--features LIST] [--cpus N]
// [--memory GiB]. Its --backend, --cpus and --memory are its own, in
// place of the root's for new VMs.
func newSetupCmd(u *ui.UI) *cobra.Command {
	var (
		features string
		c        setupChoice
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Choose the default backend, base's features and new VMs' size",
		Long: "Choose the backend new VMs get by default (lima, or smolvm, experimental), then the " +
			"optional features the base image gets: omb (oh-my-bash), fzf, cli-tools, and your own in " +
			"~/.config/devvm/features, then the CPUs and memory new VMs get. In a terminal it asks for " +
			"all three, starting from the current choices, and for the size that's what devvm size -g " +
			"saved, else base's size fitted to this machine. --backend, --features, --cpus and --memory " +
			"each choose without asking, and what they leave out stays as it is. A new choice of features " +
			"makes base and the images built on it rebuild when next used. A new size changes only the " +
			"VMs made after it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("features") {
				c.features = &features
			}
			fail(u, newSetup(u).run(c), msg.SetupFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&features, "features", "", "the `LIST` of features base gets, without asking: fzf,omb, or '' for none")
	f.StringVar(&c.backend, "backend", "", "the `NAME` of the backend new VMs get by default")
	f.StringVar(&c.size.CPUs, "cpus", "", "the `N` CPUs new VMs get, without asking")
	f.StringVar(&c.size.Memory, "memory", "", "the memory new VMs get in `GiB`, without asking: 16, 16G or 16GiB")
	return cmd
}

// setupChoice is what devvm setup's flags choose: base's features (nil
// for none given), the default backend and new VMs' CPUs and memory (""
// for none given).
type setupChoice struct {
	features *string
	backend  string
	size     vm.Size
}

// none reports whether no flag chose anything.
func (c setupChoice) none() bool {
	return c.features == nil && c.backend == "" && c.size == (vm.Size{})
}

// setup is devvm setup's run: the UI it prints through, whether it's in a
// terminal, how it asks for the backend and the size, and the host the
// size is checked against.
type setup struct {
	ui       *ui.UI
	terminal func() bool
	pickOne  func(msg.M, []ui.Choice) int
	answer   func(msg.M) string
	host     func() (vm.Host, error)
}

// newSetup returns a setup on u's terminal and prompts, on this machine.
func newSetup(u *ui.UI) setup {
	return setup{
		ui:       u,
		terminal: func() bool { return u.StdinIsTerminal() && u.StdoutIsTerminal() },
		pickOne:  u.PickOne,
		answer:   u.Answer,
		host:     vm.ThisHost,
	}
}

// run saves what c chooses: the default backend, base's features, then
// new VMs' size. With nothing chosen, it asks for all three in a
// terminal; without one, it keeps the features (every one until they're
// chosen) and saves them. A change to the features says base rebuilds,
// and that devvm gc deletes the old images.
func (s setup) run(c setupChoice) error {
	dirs, err := paths.FromEnv()
	if err != nil {
		return err
	}
	ask := c.none() && s.terminal()
	if c.backend != "" || ask {
		if err := s.saveBackend(dirs, c.backend); err != nil {
			return err
		}
	}
	if c.features != nil || c.none() {
		if err := s.saveFeatures(dirs, c.features); err != nil {
			return err
		}
	}
	if c.size == (vm.Size{}) && !ask {
		return nil
	}
	host, err := s.host()
	if err != nil {
		return err
	}
	sc := vm.SizeChoice{Dirs: dirs, Assets: assets.FS, Host: host, Answer: s.answer, Say: s.ui.Say, Warn: s.ui.Warn, Info: s.ui.Info}
	if ask {
		return sc.Ask()
	}
	return sc.Set(c.size)
}

// saveBackend saves name as the default backend, asking for it when "".
func (s setup) saveBackend(dirs paths.Dirs, name string) error {
	reg, err := registry.New(dirs, assets.FS, s.ui)
	if err != nil {
		return err
	}
	if name == "" {
		if name, err = s.pickBackend(dirs, reg); err != nil {
			return err
		}
	} else if _, ok := reg.Lookup(name); !ok {
		return msg.BackendUnknown(strings.Join(reg.Names(), ", "), name)
	}
	if err := state.WriteBackend(dirs, name); err != nil {
		return err
	}
	s.ui.Info(msg.DefaultBackend(name))
	return nil
}

// saveFeatures saves base's features: features' list, or asked for when
// nil.
func (s setup) saveFeatures(dirs paths.Dirs, features *string) error {
	b := build.Choice{Env: newBuilder(s.ui, dirs, nil, backend.Review{}, nil, "")}
	var changed bool
	var err error
	if features == nil {
		changed, err = b.PickFeatures()
	} else {
		changed, err = b.SetFeatures(splitList(*features))
	}
	if err != nil || !changed {
		return err
	}
	s.ui.Info(msg.BaseWillRebuild())
	if built, err := state.AnyImage(dirs); err != nil {
		return err
	} else if built {
		s.ui.Say(msg.GCOldHint())
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
		if registry.Experimental(n) {
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
