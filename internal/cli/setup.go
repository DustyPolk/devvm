package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// featuresNone is the value of devvm setup --features that chooses no
// features.
const featuresNone = "none"

// newSetupCmd is devvm setup [--backend NAME] [--features LIST|none]
// [--agents LIST|none]. Its --backend is its own, in place of the root's
// for new VMs, and its --agents chooses new VMs' default.
func newSetupCmd(u *ui.UI) *cobra.Command {
	var (
		features, agents string
		c                setupChoice
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Choose the default backend, base's features, and new VMs' size, network policy and coding agents",
		Long: "Choose the backend new VMs get by default, the features base gets, and new VMs' CPUs, memory, network policy and coding agents. " +
			"In a terminal devvm asks for each, starting from the current choice, and --backend, --features and --agents choose without asking. " +
			"A new choice of features rebuilds base and the images on it when they're next used.",
		Example: examples(
			"devvm setup", "answer every question", "devvm setup --features omb,fzf", "give base oh-my-bash and fzf, without asking",
			"devvm setup --features none", "give base no features",
			"devvm setup --agents claude,codex", "give new VMs Claude Code and Codex, without asking",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("features") {
				c.features = &features
			}
			if cmd.Flags().Changed("agents") {
				c.agents = partFlag(agents)
			}
			fail(u, newSetup(u).run(c), msg.SetupFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&features, "features", "", "the `LIST` of features base gets, such as fzf,omb, or none")
	f.StringVar(&c.backend, "backend", "", "the backend `NAME` new VMs get by default")
	addAgentsFlag(f, &agents, "the `LIST` of coding agents new VMs get by default, such as claude,codex, or none")
	return cmd
}

// setupChoice is what devvm setup's flags choose: base's features and new
// VMs' agents (nil for none given) and the default backend ("" for none
// given).
type setupChoice struct {
	features *string
	agents   *[]string
	backend  string
}

// none reports whether no flag chose anything.
func (c setupChoice) none() bool {
	return c.features == nil && c.agents == nil && c.backend == ""
}

// setup is devvm setup's run: the UI it prints through, whether it's in a
// terminal, how it asks for the backend, the size, the network policy and
// the agents, and the host the size is checked against.
type setup struct {
	ui       *ui.UI
	terminal func() bool
	pickOne  func(msg.M, []ui.Choice) int
	pick     func(msg.M, []vm.PickPart) []vm.PickPart
	answer   func(msg.M) string
	host     func() (vm.Host, error)
}

// newSetup returns a setup on u's terminal and prompts, on this machine.
func newSetup(u *ui.UI) setup {
	return setup{
		ui:       u,
		terminal: func() bool { return u.StdinIsTerminal() && u.StdoutIsTerminal() },
		pickOne:  u.PickOne,
		pick:     pickParts(u),
		answer:   u.Answer,
		host:     func() (vm.Host, error) { return vm.ThisHost(thisHost()) },
	}
}

// run saves what c chooses: the default backend, base's features, then
// new VMs' agents. With nothing chosen, it asks in a terminal for those,
// with new VMs' size and network policy before the agents; without one,
// it keeps the features (every one until they're chosen) and saves them.
// A change to the features says base rebuilds, and that devvm gc deletes
// the old images. What config.toml sets isn't asked for, said, and a flag
// choosing it is refused (set_in_config).
func (s setup) run(c setupChoice) error {
	dirs, err := paths.FromEnv()
	if err != nil {
		return err
	}
	def, err := settings.Read(dirs)
	if err != nil {
		return err
	}
	ask := c.none() && s.terminal()
	if c.backend != "" || ask {
		if err := s.saveBackend(dirs, def, c.backend); err != nil {
			return err
		}
	}
	if c.features != nil || c.none() {
		if err := s.saveFeatures(dirs, def, c.features); err != nil {
			return err
		}
	}
	agents := vm.AgentsChoice{Dirs: dirs, Pick: s.pick, Info: s.ui.Info}
	if c.agents != nil {
		return agents.Set(*c.agents)
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
	if err := s.saveNetwork(dirs); err != nil {
		return err
	}
	return agents.Ask()
}

// inConfig reports whether config.toml sets key, saying devvm setup
// doesn't ask for it when it does.
func (s setup) inConfig(dirs paths.Dirs, def settings.Defaults, key string) bool {
	if def.Source(key) != settings.FromConfig {
		return false
	}
	s.ui.Info(msg.SetupInConfig(dirs.ConfigFile(), key))
	return true
}

// saveNetwork asks which network policy new VMs get, starting from the
// one in effect, and saves it (vm.NetworkChoice), unless config.toml sets
// it.
func (s setup) saveNetwork(dirs paths.Dirs) error {
	def, err := settings.Read(dirs)
	if err != nil || s.inConfig(dirs, def, settings.KeyNetwork) {
		return err
	}
	modes := settings.NetworkChoices()
	descs := map[string]msg.M{
		settings.NetworkAsk: msg.NetworkDefaultAsk(),
		settings.NetworkOn:  msg.NetworkDefaultOn(),
		settings.NetworkOff: msg.NetworkDefaultOff(),
	}
	choices := make([]ui.Choice, len(modes))
	for i, m := range modes {
		choices[i] = ui.Choice{Name: m, Desc: descs[m].String(), On: m == def.Network.Value}
	}
	mode := modes[s.pickOne(msg.NetworkDefaultHelp(), choices)]
	return vm.NetworkChoice{Dirs: dirs, Info: s.ui.Info}.Set(mode)
}

// saveBackend saves name as the default backend, asking for it when "",
// unless config.toml sets it.
func (s setup) saveBackend(dirs paths.Dirs, def settings.Defaults, name string) error {
	if name == "" && s.inConfig(dirs, def, settings.KeyBackend) {
		return nil
	}
	reg, err := newRegistry(dirs, assets.FS, s.ui)
	if err != nil {
		return err
	}
	if name == "" {
		name = s.pickBackend(reg, def.Backend.Value)
	} else if _, ok := reg.Lookup(name); !ok {
		return msg.BackendUnknown(strings.Join(reg.Names(), ", "), name)
	}
	if _, err := settings.Save(dirs, settings.Values{Backend: &name}); err != nil {
		return err
	}
	s.ui.Info(msg.DefaultBackend(name))
	return nil
}

// saveFeatures saves base's features: features' list, or asked for when
// nil, unless config.toml sets them.
func (s setup) saveFeatures(dirs paths.Dirs, def settings.Defaults, features *string) error {
	if features == nil && s.inConfig(dirs, def, settings.KeyFeatures) {
		return nil
	}
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
// current, the one in effect (backend.Default when it's one devvm doesn't
// have).
func (s setup) pickBackend(reg *backend.Registry, current string) string {
	if _, ok := reg.Lookup(current); !ok {
		current = backend.Default
	}
	names := reg.Names()
	choices := make([]ui.Choice, len(names))
	for i, n := range names {
		choices[i] = ui.Choice{Name: n, On: n == current}
	}
	return names[s.pickOne(msg.BackendHelp(), choices)]
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
