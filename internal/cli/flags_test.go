package cli

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/w3cj/devvm/internal/msg"
)

// eachCommand calls do on c and every command under it.
func eachCommand(c *cobra.Command, do func(*cobra.Command)) {
	do(c)
	for _, sub := range c.Commands() {
		eachCommand(sub, do)
	}
}

// Flags are local unless truly global: every command takes -h/--help,
// --no-input and -v/--verbose, and no command has another persistent
// flag, so no subcommand takes the root's flags for a new VM.
func TestPersistentFlags(t *testing.T) {
	persistentFlags := map[string]string{"help": "h", "no-input": "", "verbose": "v"}
	u, _, _ := testUI()
	root := NewRoot(u)
	eachCommand(root, func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if _, ok := persistentFlags[f.Name]; !ok {
				t.Errorf("%s: --%s is persistent", c.CommandPath(), f.Name)
			}
		})
		for name, short := range persistentFlags {
			f := c.LocalFlags().Lookup(name)
			if f == nil {
				f = c.InheritedFlags().Lookup(name)
			}
			if f == nil || f.Shorthand != short {
				t.Errorf("%s: --%s is %+v, want one with shorthand %q", c.CommandPath(), name, f, short)
			}
		}
	})
}

// Every shorthand is one of shorthands, for its flag; no command takes
// --yes, as every question has a flag of its own that answers it; and a
// flag has one help text wherever it appears, but for manyUses and
// Cobra's own --help.
func TestFlagNames(t *testing.T) {
	// the only one-letter flags, each for one flag name wherever it
	// appears: few enough to learn
	shorthands := map[string]string{"h": "help", "v": "verbose", "f": "force", "g": "global", "n": "lines"}
	// the flag names whose help differs between commands, with why it's
	// one meaning still
	manyUses := map[string]string{
		"cpus":   "a new VM's says where its default comes from, and devvm size's changes the VM's",
		"memory": "a new VM's says where its default comes from, and devvm size's changes the VM's",
		"disk":   "a new VM's is at least the image's, and devvm size's at least what the VM has",
		"global": "each setting's -g says which setting new VMs get",
		"json":   "devvm doctor's also says it succeeds whatever is missing",
		"as":     "the name a VM reaches another by: devvm link add's for the link's target, devvm group join's for this VM in the others",
		"agents": "the coding agents a list of them gives: a new VM's, or with devvm setup new VMs' default",
	}
	u, _, _ := testUI()
	root := NewRoot(u)
	usages := map[string]map[string]bool{}
	eachCommand(root, func(c *cobra.Command) {
		if c.Hidden {
			return
		}
		c.InitDefaultHelpFlag()
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Shorthand != "" && shorthands[f.Shorthand] != f.Name {
				t.Errorf("%s: -%s is --%s: give it no shorthand, or add it to shorthands", c.CommandPath(), f.Shorthand, f.Name)
			}
			if f.Name == "yes" {
				t.Errorf("%s takes --yes: give the question it answers a flag of its own", c.CommandPath())
			}
			if f.Name == "help" {
				return
			}
			if usages[f.Name] == nil {
				usages[f.Name] = map[string]bool{}
			}
			usages[f.Name][f.Usage] = true
		})
	})
	for name, texts := range usages {
		_, many := manyUses[name]
		switch {
		case len(texts) > 1 && !many:
			t.Errorf("--%s has %d help texts, want one: %q", name, len(texts), slices.Sorted(maps.Keys(texts)))
		case len(texts) == 1 && many:
			t.Errorf("--%s has one help text: take it out of manyUses", name)
		}
	}
}

// A flag whose value is a DURATION takes durations' one grammar,
// state.ParseDuration's, checked before the command runs; none is Go's
// time.Duration, whose grammar differs.
func TestDurationFlags(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	eachCommand(root, func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "duration" {
				t.Errorf("%s: --%s is a time.Duration", c.CommandPath(), f.Name)
			}
			if flagForm(f) != "DURATION" {
				return
			}
			if err := checkFlagValue(f.Name, "30d"); err != nil {
				t.Errorf("--%s 30d: %v", f.Name, err)
			}
			err := checkFlagValue(f.Name, "30")
			if msg.IDOf(err) != "bad_duration" {
				t.Errorf("--%s 30: %v, want bad_duration: check it with state.ParseDuration in checkFlagValue", f.Name, err)
			}
		})
	})
}

// flagUse is what a flag name must keep the same on every command: one
// name, one meaning.
type flagUse struct {
	shorthand, kind, value string
	where                  string
}

// A flag name has one meaning across the tree: the same shorthand, type
// and value it takes wherever it appears, and a shorthand names one flag.
func TestFlagNamesHaveOneMeaning(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	byName := map[string]flagUse{}
	byShort := map[string]string{}
	eachCommand(root, func(c *cobra.Command) {
		if c.Hidden {
			return
		}
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			value, _ := pflag.UnquoteUsage(f)
			got := flagUse{shorthand: f.Shorthand, kind: f.Value.Type(), value: value, where: c.CommandPath()}
			if was, ok := byName[f.Name]; ok {
				if was.shorthand != got.shorthand || was.kind != got.kind || was.value != got.value {
					t.Errorf("--%s: %+v on %s, %+v on %s", f.Name, got, got.where, was, was.where)
				}
			} else {
				byName[f.Name] = got
			}
			if f.Shorthand == "" {
				return
			}
			if name, ok := byShort[f.Shorthand]; ok && name != f.Name {
				t.Errorf("-%s is --%s on %s and --%s elsewhere", f.Shorthand, f.Name, c.CommandPath(), name)
			}
			byShort[f.Shorthand] = f.Name
		})
	})
}

// --tools, --agents and --layers each take none alone or a list of their
// part's names, the user's layers among the layers; anything else is
// refused, with a did-you-mean when a name is close. As flags, each
// replaces its part, none with nothing, and left out it's nil.
func TestPartFlags(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	writeLayer(t, cfg, "mine")
	for _, tc := range []struct{ flag, value, want string }{
		{"tools", "node,go", ""},
		{"tools", "none", ""},
		{"tools", "claude", "part_other_flag flag=--tools given=claude other=--agents"},
		{"tools", "node,mine", "part_other_flag flag=--tools given=mine other=--layers"},
		{"tools", "java", "unknown_part flag=--tools given=java names=node python rust go"},
		{"tools", "pyhton", "unknown_part_like flag=--tools given=pyhton like=python"},
		{"tools", "node,none", "part_none_alone flag=--tools"},
		{"tools", ",", "part_none_alone flag=--tools"},
		{"agents", "claude,opencode", ""},
		{"agents", "cladue", "unknown_part_like flag=--agents given=cladue like=claude"},
		{"agents", "node", "part_other_flag flag=--agents given=node other=--tools"},
		{"layers", "docker,mine", ""},
		{"layers", "nix", ""},
		{"layers", "desktop", ""},
		{"layers", "base", "unknown_part flag=--layers given=base names=desktop docker mine nix"},
		{"layers", "node", "part_other_flag flag=--layers given=node other=--tools"},
		{"layers", "codex", "part_other_flag flag=--layers given=codex other=--agents"},
		{"layers", "mien", "unknown_part_like flag=--layers given=mien like=mine"},
	} {
		if got := msg.NamesOf(checkFlagValue(tc.flag, tc.value)); got != tc.want {
			t.Errorf("--%s %s: %q, want %q", tc.flag, tc.value, got, tc.want)
		}
	}
	if partFlag("") != nil || len(*partFlag("none")) != 0 || !slices.Equal(*partFlag("node,go"), []string{"node", "go"}) {
		t.Error("partFlag")
	}
}

// writeLayer writes a layer of the user's under the config folder cfg.
func writeLayer(t *testing.T, cfg, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(cfg, "devvm", "layers"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "devvm", "layers", name+".sh"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// devvm's flags are its own, --cpus, --memory and --disk are only its and
// devvm size's, and --agents only its and devvm setup's.
func TestUpFlagsAreUpsAlone(t *testing.T) {
	u, _, _ := testUI()
	root := NewRoot(u)
	sizeFlags := []string{"cpus", "memory", "disk"}
	upOnly := []string{
		"network-policy", "no-network-policy", "ports", "clone", "here", "unsafe-folder", "no-git-protect", "ssh-agent", "no-devcontainer",
		"tools", "agents", "layers",
	}
	eachCommand(root, func(c *cobra.Command) {
		for _, name := range slices.Concat(sizeFlags, upOnly) {
			if c.Flags().Lookup(name) == nil {
				continue
			}
			ok := c == root || (c.Name() == "size" && slices.Contains(sizeFlags, name)) || (c.Name() == "setup" && name == "agents")
			if !ok {
				t.Errorf("%s takes --%s", c.CommandPath(), name)
			}
		}
	})
}
