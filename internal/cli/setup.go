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

// featuresNone is the value of devvm setup --features that chooses no
// features.
const featuresNone = "none"

// newSetupCmd is devvm setup [--backend NAME] [--features LIST|none]. Its
// --backend is its own, in place of the root's for new VMs.
func newSetupCmd(u *ui.UI) *cobra.Command {
	var (
		features string
		c        setupChoice
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Choose the default backend, base's features, and new VMs' size and network policy",
		Long: "Choose the backend new VMs get by default, the features base gets, and new VMs' CPUs, memory and network policy. " +
			"In a terminal devvm asks for each, starting from the current choice, and --backend and --features choose without asking. " +
			"A new choice of features rebuilds base and the images on it when they're next used.",
		Example: examples(
			"devvm setup", "answer every question",
			"devvm setup --backend smolvm", "make smolvm the default backend, without asking",
			"devvm setup --features omb,fzf", "give base oh-my-bash and fzf, without asking",
			"devvm setup --features none", "give base no features",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("features") {
				c.features = &features
			}
			fail(u, newSetup(u).run(c), msg.SetupFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&features, "features", "", "the `LIST` of features base gets, such as fzf,omb, or none")
	f.StringVar(&c.backend, "backend", "", "the backend `NAME` new VMs get by default")
	return cmd
}

// setupChoice is what devvm setup's flags choose: base's features (nil
// for none given) and the default backend ("" for none given).
type setupChoice struct {
	features *string
	backend  string
}

// none reports whether no flag chose anything.
func (c setupChoice) none() bool {
	return c.features == nil && c.backend == ""
}

// setup is devvm setup's run: the UI it prints through, whether it's in a
// terminal, how it asks for the backend, the size and the network policy,
// and the host the size is checked against.
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
		host:     func() (vm.Host, error) { return vm.ThisHost(thisHost()) },
	}
}

// run saves what c chooses: the default backend, then base's features.
// With nothing chosen, it asks in a terminal for those, then new VMs' size
// and network policy; without one, it keeps the features (every one until
// they're chosen) and saves them. A change to the features says base
// rebuilds, and that devvm gc deletes the old images.
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
	if !ask {
		return nil
	}
	host, err := s.host()
	if err != nil {
		return err
	}
	sc := vm.SizeChoice{Dirs: dirs, Assets: assets.FS, Host: host, Answer: s.answer, Say: s.ui.Say, Warn: s.ui.Warn, Info: s.ui.Info}
	if err := sc.Ask(); err != nil {
		return err
	}
	return s.saveNetwork(dirs)
}

// saveNetwork asks which network policy new VMs get, starting from the
// saved one, and saves it in the network file new VMs read.
func (s setup) saveNetwork(dirs paths.Dirs) error {
	current, err := state.ReadNetwork(dirs)
	if err != nil {
		return err
	}
	modes := state.NetworkChoices()
	descs := map[string]msg.M{
		state.NetworkAsk: msg.NetworkDefaultAsk(),
		state.NetworkOn:  msg.NetworkDefaultOn(),
		state.NetworkOff: msg.NetworkDefaultOff(),
	}
	choices := make([]ui.Choice, len(modes))
	for i, m := range modes {
		choices[i] = ui.Choice{Name: m, Desc: descs[m].String(), On: m == current}
	}
	mode := modes[s.pickOne(msg.NetworkDefaultHelp(), choices)]
	if err := state.WriteNetwork(dirs, mode); err != nil {
		return err
	}
	s.ui.Info(msg.NetworkDefault(mode))
	return nil
}

// saveBackend saves name as the default backend, asking for it when "".
func (s setup) saveBackend(dirs paths.Dirs, name string) error {
	reg, err := newRegistry(dirs, assets.FS, s.ui)
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
	switch {
	case features == nil:
		changed, err = b.PickFeatures()
	case *features == featuresNone:
		changed, err = b.SetFeatures(nil)
	default:
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
