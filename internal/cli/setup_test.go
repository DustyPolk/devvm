package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// asked is what setup's backend picker was shown.
type asked struct {
	help    string
	choices []ui.Choice
}

// setupSandbox points devvm's folders at a temporary HOME, with a feature
// of the user's, and returns a setup that sees a terminal (or not) and
// picks the backend answer names, keeping the size with Enter and picking
// network policy off, on a host of 8 CPUs and 32 GiB; the folders; what
// the backend picker was shown, nil until it's asked; and the size
// prompts and network pickers shown.
func setupSandbox(t *testing.T, terminal bool, answer string) (setup, paths.Dirs, func() *asked, *[]string) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("DEVVM_LIMA_HOME", "")
	d, err := paths.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(d.Features(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.Features(), "mine.sh"), []byte("#!/bin/bash\n# mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var a *asked
	var sized []string
	u, _, _ := testUI()
	s := setup{
		ui:       u,
		terminal: func() bool { return terminal },
		pickOne: func(help msg.M, cs []ui.Choice) int {
			if help.ID() == "network_default_help" {
				sized = append(sized, pickedNetwork(cs))
				return slices.IndexFunc(cs, func(c ui.Choice) bool { return c.Name == settings.NetworkOff })
			}
			if a != nil {
				t.Error("asked for the backend twice")
			}
			a = &asked{help.String(), slices.Clone(cs)}
			return slices.IndexFunc(cs, func(c ui.Choice) bool { return c.Name == answer })
		},
		// The agents' question checks Claude Code.
		pick: func(help msg.M, parts []vm.PickPart) []vm.PickPart {
			sized = append(sized, help.ID())
			parts[0].Rows[0].On = true
			return parts
		},
		answer: func(m msg.M) string { sized = append(sized, m.Names()); return "" },
		host:   func() (vm.Host, error) { return vm.Host{CPUs: 8, MemoryGiB: 32}, nil },
	}
	return s, d, func() *asked { return a }, &sized
}

// defaults reads the defaults in effect in d.
func defaults(t *testing.T, d paths.Dirs) settings.Defaults {
	t.Helper()
	def, err := settings.Read(d)
	if err != nil {
		t.Fatal(err)
	}
	return def
}

// save saves a default as devvm's commands do.
func save(t *testing.T, d paths.Dirs, v settings.Values) {
	t.Helper()
	if _, err := settings.Save(d, v); err != nil {
		t.Fatal(err)
	}
}

// devvm setup --agents sets new VMs' agents without asking, none for
// none; in a terminal without flags, it asks for them last.
func TestSetupAgents(t *testing.T) {
	s, d, _, asked := setupSandbox(t, true, "lima")
	for _, tc := range []struct {
		value string
		want  []string
	}{{"codex,claude", []string{"claude", "codex"}}, {"none", []string{}}} {
		if err := s.run(setupChoice{agents: partFlag(tc.value)}); err != nil {
			t.Fatal(err)
		}
		if got := defaults(t, d).Agents; !got.Chosen || !slices.Equal(got.Names, tc.want) {
			t.Errorf("--agents %s: saved %+v", tc.value, got)
		}
	}
	if len(*asked) > 0 {
		t.Errorf("asked %q", *asked)
	}
	if err := s.run(setupChoice{}); err != nil {
		t.Fatal(err)
	}
	if n := len(*asked); n == 0 || (*asked)[n-1] != "agents_help" {
		t.Errorf("asked %q, want agents_help last", *asked)
	}
	if got := defaults(t, d).Agents.Names; !slices.Equal(got, []string{"claude"}) {
		t.Errorf("saved %q", got)
	}
}

// pickedNetwork is the network picker's choices, as names, the one it
// starts on starred, and their descriptions' message names.
func pickedNetwork(cs []ui.Choice) string {
	s := "network"
	for _, c := range cs {
		on := ""
		if c.On {
			on = "*"
		}
		s += " " + on + c.Name + ":" + c.Desc
	}
	return s
}

// The network picker starts from the network policy saved for new VMs.
func TestSetupNetworkStartsFromSaved(t *testing.T) {
	s, d, _, picked := setupSandbox(t, true, "lima")
	save(t, d, settings.Values{Network: settings.Of(settings.NetworkOn)})
	if err := s.run(setupChoice{}); err != nil {
		t.Fatal(err)
	}
	want := "network ask:network_default_ask *on:network_default_on off:network_default_off"
	if !slices.Contains(*picked, want) {
		t.Errorf("pickers %q, want %q", *picked, want)
	}
	if mode := defaults(t, d).Network; mode != (settings.Value{Value: settings.NetworkOff, Source: settings.FromSettings}) {
		t.Errorf("network policy for new VMs %+v", mode)
	}
}

// savedBackend is the default backend devvm saved, "" without one.
func savedBackend(t *testing.T, d paths.Dirs) string {
	t.Helper()
	if v := defaults(t, d).Backend; v.Source == settings.FromSettings {
		return v.Value
	}
	return ""
}

// In a terminal, setup with neither flag asks for the backend (starting
// from the current one, smolvm marked experimental), then the features;
// --backend, --features or no terminal ask no backend.
func TestSetupAsksForTheBackend(t *testing.T) {
	none := ""
	for _, tc := range []struct {
		name     string
		terminal bool
		flag     string
		features *string
		current  string // the default backend saved; "" for none
		ask      bool
		want     string // the default backend saved afterwards
		start    string // the choice the picker starts on
		chosen   bool   // base's features chosen
	}{
		{name: "a terminal, nothing set", terminal: true, ask: true, start: "lima", want: "smolvm", chosen: true},
		{name: "a terminal, smolvm set", terminal: true, current: "smolvm", ask: true, start: "smolvm", want: "smolvm", chosen: true},
		{name: "a terminal, an unknown one set", terminal: true, current: "qemu", ask: true, start: "lima", want: "smolvm", chosen: true},
		{name: "no terminal", want: "", chosen: true},
		{name: "--backend", terminal: true, flag: "lima", want: "lima"},
		{name: "--features", terminal: true, features: &none, current: "lima", want: "lima", chosen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, shown, sized := setupSandbox(t, tc.terminal, "smolvm")
			if tc.current != "" {
				save(t, d, settings.Values{Backend: &tc.current})
			}
			if err := s.run(setupChoice{features: tc.features, backend: tc.flag}); err != nil {
				t.Fatal(err)
			}
			wantSized := []string(nil)
			if tc.ask {
				wantSized = []string{
					"size_cpus_prompt cpus=4", "size_memory_prompt memory=8",
					"network *ask:network_default_ask on:network_default_on off:network_default_off",
					"agents_help",
				}
			}
			if !slices.Equal(*sized, wantSized) {
				t.Errorf("size prompts and network pickers %q, want %q", *sized, wantSized)
			}
			def := defaults(t, d)
			if mode := def.Network.Value; mode != map[bool]string{true: settings.NetworkOff, false: settings.NetworkAsk}[tc.ask] {
				t.Errorf("network policy for new VMs %q", mode)
			}
			if def.CPUs.Source != settings.FromBuiltin || def.Memory.Source != settings.FromBuiltin {
				t.Errorf("Enter saved a size: %+v %+v", def.CPUs, def.Memory)
			}
			if a := shown(); (a != nil) != tc.ask {
				t.Fatalf("asked %v, want %v", a != nil, tc.ask)
			} else if a != nil {
				want := []ui.Choice{
					{Name: "docker", Desc: "backend_experimental", On: tc.start == "docker"},
					{Name: "lima", On: tc.start == "lima"},
					{Name: "smolvm", Desc: "backend_experimental", On: tc.start == "smolvm"},
				}
				if a.help != "backend_help" || !slices.Equal(a.choices, want) {
					t.Errorf("shown %q %+v, want backend_help %+v", a.help, a.choices, want)
				}
			}
			if got := savedBackend(t, d); got != tc.want {
				t.Errorf("default backend %q, want %q", got, tc.want)
			}
			if def.Features.Chosen != tc.chosen {
				t.Errorf("base's features chosen %v, want %v", def.Features.Chosen, tc.chosen)
			}
		})
	}
}

// What config.toml sets, devvm setup doesn't ask for, saying so, and its
// flags can't choose (set_in_config); it still asks for the rest.
func TestSetupConfigSets(t *testing.T) {
	s, d, shown, sized := setupSandbox(t, true, "smolvm")
	u, said, _ := testUI()
	s.ui = u
	config := "backend = \"docker\"\nnetwork = \"on\"\nfeatures = []\nagents = [\"codex\"]\n[size]\ncpus = 2\n"
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ConfigFile(), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.run(setupChoice{}); err != nil {
		t.Fatal(err)
	}
	if shown() != nil || !slices.Equal(*sized, []string{"size_memory_prompt memory=8"}) {
		t.Errorf("asked %+v, %q", shown(), *sized)
	}
	for _, key := range []string{"backend", "features", "size.cpus", "network", "agents"} {
		if want := "setup_in_config file=" + d.ConfigFile() + " key=" + key; !strings.Contains(said.String(), want) {
			t.Errorf("said\n%s\nwant %s", said, want)
		}
	}
	if _, err := os.Stat(d.Settings()); err == nil {
		t.Error("saved settings.json")
	}
	for _, c := range []setupChoice{{backend: "lima"}, {features: &[]string{"none"}[0]}, {agents: partFlag("claude")}} {
		if err := s.run(c); msg.IDOf(err) != "set_in_config" {
			t.Errorf("%+v: %v", c, msg.NamesOf(err))
		}
	}
}
