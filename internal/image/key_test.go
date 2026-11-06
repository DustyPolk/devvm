package image

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/devvmhq/devvm/assets"
)

// input is an image's key input with every field set.
func input() Input {
	return Input{
		Backend: "lima", Image: "foo", Parent: "0123456789ab", From: "node:24", FromDigest: "node@sha256:aaaa",
		Recipe: Recipe{Origin: []Step{{Name: "base.yaml", Data: []byte("cpus: 4\n")}}, Steps: []Step{
			{Name: "foo.sh", Data: []byte("echo foo\n"), Args: []string{"a", "b"}},
			{Name: "bar.sh", Data: []byte("echo bar\n"), Root: true},
		}},
		Missing: "baz.sh", Rebuild: "1",
	}
}

// Each field of a key's input, and each part of each step, changes the
// key; the same input is the same key, 12 hex digits.
func TestKeyChangesWithEachField(t *testing.T) {
	first := input().Key()
	if !IsKey(first) || input().Key() != first {
		t.Fatalf("key %q, then %q", first, input().Key())
	}
	for name, change := range map[string]func(in *Input){
		"the backend":       func(in *Input) { in.Backend = "qemu" },
		"the name":          func(in *Input) { in.Image = "bar" },
		"the parent":        func(in *Input) { in.Parent = "ba9876543210" },
		"--from":            func(in *Input) { in.From = "node:25" },
		"--from's digest":   func(in *Input) { in.FromDigest = "node@sha256:bbbb" },
		"an origin step":    func(in *Input) { in.Recipe.Origin[0].Data = []byte("cpus: 2\n") },
		"no origin":         func(in *Input) { in.Recipe.Origin = nil },
		"a step's name":     func(in *Input) { in.Recipe.Steps[0].Name = "x.sh" },
		"a step's script":   func(in *Input) { in.Recipe.Steps[0].Data = []byte("echo x\n") },
		"a step's argument": func(in *Input) { in.Recipe.Steps[0].Args = []string{"a", "c"} },
		"a step more":       func(in *Input) { in.Recipe.Steps = append(in.Recipe.Steps, Step{}) },
		"the steps' order":  func(in *Input) { slices.Reverse(in.Recipe.Steps) },
		"a step as root":    func(in *Input) { in.Recipe.Steps[0].Root = true },
		"a step not root":   func(in *Input) { in.Recipe.Steps[1].Root = false },
		"what's missing":    func(in *Input) { in.Missing = "" },
		"the rebuild":       func(in *Input) { in.Rebuild = "2" },
	} {
		in := input()
		change(&in)
		if in.Key() == first {
			t.Errorf("%s: the same key", name)
		}
	}
}

// Moving bytes from one field to the next changes the key: the fields
// aren't simply joined.
func TestKeyFieldsAreSeparate(t *testing.T) {
	steps := func(ss ...Step) string { return Input{Backend: "lima", Image: Base, Recipe: Recipe{Steps: ss}}.Key() }
	for name, pair := range map[string][2]string{
		"a line from one script to the next": {
			steps(Step{Name: "a.sh", Data: []byte("x\n")}, Step{Name: "b.sh", Data: []byte("y\n")}),
			steps(Step{Name: "a.sh", Data: []byte("x\ny\n")}, Step{Name: "b.sh"}),
		},
		"a step's name and its script": {
			steps(Step{Name: "a", Data: []byte("b")}), steps(Step{Name: "ab"}),
		},
		"one argument or two": {
			steps(Step{Args: []string{"a b"}}), steps(Step{Args: []string{"a", "b"}}),
		},
		"an argument or the script": {
			steps(Step{Data: []byte("a"), Args: []string{}}), steps(Step{Args: []string{"a"}}),
		},
		"no step or an empty one": {steps(), steps(Step{})},
	} {
		if pair[0] == pair[1] {
			t.Errorf("%s: the same key", name)
		}
	}
	// The same step as the backend's origin or as one of base's own.
	s := Step{Name: "a.sh"}
	if (Input{Backend: "lima", Image: Base, Recipe: Recipe{Origin: []Step{s}}}).Key() == steps(s) {
		t.Error("an origin step and a step have the same key")
	}
}

func TestChainString(t *testing.T) {
	c := Chain{{"base", "0123456789ab"}, {"foo", "ba9876543210"}}
	if s := c.String(); s != "base=0123456789ab foo=ba9876543210" {
		t.Errorf("String: %q", s)
	}
	if c.Key() != "ba9876543210" || Chain(nil).Key() != "" {
		t.Errorf("Key: %q", c.Key())
	}
}

// The built-in layers are desktop, docker and nix, sorted, each a script
// in devvm's own layers/, and none of them is one of base's features.
func TestBuiltins(t *testing.T) {
	if want := []string{"desktop", "docker", "nix"}; !slices.Equal(Builtins, want) {
		t.Errorf("Builtins = %q, want %q", Builtins, want)
	}
	for _, name := range Builtins {
		if !IsBuiltin(name) || !LayerName(name) {
			t.Errorf("%s: IsBuiltin %v, LayerName %v", name, IsBuiltin(name), LayerName(name))
		}
		if _, err := fs.Stat(assets.FS, "layers/"+name+".sh"); err != nil {
			t.Errorf("%s has no script: %v", name, err)
		}
		if _, err := fs.Stat(assets.FS, "features/"+name+".sh"); err == nil {
			t.Errorf("%s is one of base's features too", name)
		}
	}
	if IsBuiltin("base") || IsBuiltin("mine") {
		t.Error("IsBuiltin took base or a layer of the user's")
	}
}

func TestParseChain(t *testing.T) {
	c := Chain{{"base", "0123456789ab"}, {"foo", "ba9876543210"}}
	if got := ParseChain(c.String()); !slices.Equal(got, c) {
		t.Errorf("ParseChain(%q) = %v", c.String(), got)
	}
	if got := ParseChain("  base=0123456789ab junk =x y= foo=ba9876543210 "); !slices.Equal(got, c) {
		t.Errorf("ParseChain skipping junk = %v", got)
	}
	if got := ParseChain(""); got != nil {
		t.Errorf("ParseChain(\"\") = %v", got)
	}
}

func TestLayerName(t *testing.T) {
	for name, want := range map[string]bool{
		"foo": true, "my-layer2": true, "0x": true,
		"base": false, "": false, "-x": false, "Foo": false, "a_b": false, "a+b": false, "../x": false,
		// Shared tools' names aren't layers'.
		"node": false, "go": false, "claude": false, "pnpm": true,
		// Command names are layers' too: devvm takes no image as a word.
		"build": true, "ls": true,
	} {
		if got := LayerName(name); got != want {
			t.Errorf("LayerName(%q) = %v", name, got)
		}
	}
}
