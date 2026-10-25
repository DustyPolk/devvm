package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// asked is what setup's backend picker was shown.
type asked struct {
	help    string
	choices []ui.Choice
}

// setupSandbox points devvm's folders at a temporary HOME, with a feature
// of the user's, and makes setup see a terminal (or not) and pick the
// backend answer names. It returns the folders and what the picker was
// shown: nil until it's asked.
func setupSandbox(t *testing.T, terminal bool, answer string) (paths.Dirs, func() *asked) {
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
	oldTerm, oldPick := setupTerminal, pickOne
	setupTerminal = func() bool { return terminal }
	pickOne = func(help msg.M, cs []ui.Choice) int {
		if a != nil {
			t.Error("asked for the backend twice")
		}
		a = &asked{help.String(), slices.Clone(cs)}
		return slices.IndexFunc(cs, func(c ui.Choice) bool { return c.Name == answer })
	}
	t.Cleanup(func() { setupTerminal, pickOne = oldTerm, oldPick })
	return d, func() *asked { return a }
}

// readBackend is the default backend file's name, "" without one.
func readBackend(t *testing.T, d paths.Dirs) string {
	t.Helper()
	name, err := state.ReadBackend(d)
	if err != nil {
		t.Fatal(err)
	}
	return name
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
		current  string // the default backend file's; "" for none
		ask      bool
		want     string // the default backend file's afterwards
		start    string // the choice the picker starts on
		chosen   bool   // base-features written
	}{
		{name: "a terminal, nothing set", terminal: true, ask: true, start: "lima", want: "smolvm", chosen: true},
		{name: "a terminal, smolvm set", terminal: true, current: "smolvm", ask: true, start: "smolvm", want: "smolvm", chosen: true},
		{name: "a terminal, an unknown one set", terminal: true, current: "qemu", ask: true, start: "lima", want: "smolvm", chosen: true},
		{name: "no terminal", want: "", chosen: true},
		{name: "--backend", terminal: true, flag: "lima", want: "lima"},
		{name: "--features", terminal: true, features: &none, current: "lima", want: "lima", chosen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, shown := setupSandbox(t, tc.terminal, "smolvm")
			if tc.current != "" {
				if err := state.WriteBackend(d, tc.current); err != nil {
					t.Fatal(err)
				}
			}
			if err := setup(tc.features, tc.flag); err != nil {
				t.Fatal(err)
			}
			if a := shown(); (a != nil) != tc.ask {
				t.Fatalf("asked %v, want %v", a != nil, tc.ask)
			} else if a != nil {
				want := []ui.Choice{{Name: "lima", On: tc.start == "lima"}, {Name: "smolvm", Desc: "backend_experimental", On: tc.start == "smolvm"}}
				if a.help != "backend_help" || !slices.Equal(a.choices, want) {
					t.Errorf("shown %q %+v, want backend_help %+v", a.help, a.choices, want)
				}
			}
			if got := readBackend(t, d); got != tc.want {
				t.Errorf("default backend %q, want %q", got, tc.want)
			}
			_, err := os.Stat(d.BaseFeatures())
			if written := !errors.Is(err, fs.ErrNotExist); written != tc.chosen {
				t.Errorf("base-features written %v, want %v", written, tc.chosen)
			}
		})
	}
}
