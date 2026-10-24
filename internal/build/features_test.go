package build

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/state"
)

// featureBuilder is testBuilder with devvm's features fzf and omb, and the
// user's mine and fzf (which devvm's hides).
func featureBuilder(t *testing.T) *Builder {
	t.Helper()
	b, _ := testBuilder(t)
	write(t, filepath.Join(b.Root, "features", "omb.sh"), "#!/bin/bash\n# oh-my-bash: a prompt\n")
	write(t, filepath.Join(b.Root, "features", "fzf.sh"), "#!/bin/bash\n#fuzzy search  \n")
	write(t, filepath.Join(b.Dirs.Features(), "mine.sh"), "#!/bin/bash\n# my tools\n")
	write(t, filepath.Join(b.Dirs.Features(), "fzf.sh"), "#!/bin/bash\n# mine\n")
	write(t, filepath.Join(b.Dirs.Features(), "Bad_Name.sh"), "")
	write(t, filepath.Join(b.Dirs.Features(), "notes.txt"), "")
	if err := os.Mkdir(filepath.Join(b.Dirs.Features(), "dir.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	return b
}

// devvm's features come first, then the user's; one of the user's named
// like one of devvm's is ignored, and devvm's script is the one used.
func TestFeatures(t *testing.T) {
	b := featureBuilder(t)
	features, shadowed := b.Features()
	if want := []string{"fzf", "omb", "mine"}; !slices.Equal(features, want) {
		t.Errorf("Features() = %q, want %q", features, want)
	}
	if want := []string{filepath.Join(b.Dirs.Features(), "fzf.sh")}; !slices.Equal(shadowed, want) {
		t.Errorf("shadowed = %q, want %q", shadowed, want)
	}
	for name, want := range map[string]string{
		"fzf":  filepath.Join(b.Root, "features", "fzf.sh"),
		"mine": filepath.Join(b.Dirs.Features(), "mine.sh"),
		"gone": filepath.Join(b.Dirs.Features(), "gone.sh"),
	} {
		if got := b.FeatureScript(name); got != want {
			t.Errorf("FeatureScript(%s) = %s, want %s", name, got, want)
		}
	}
}

// A feature's description is its script's second line, a comment.
func TestDescription(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, script, want string }{
		{"comment", "#!/bin/bash\n# oh-my-bash: a prompt\nset -e\n", "oh-my-bash: a prompt"},
		{"no space", "#!/bin/bash\n#fuzzy  \n", "fuzzy"},
		{"not a comment", "#!/bin/bash\nset -e\n", ""},
		{"one line", "#!/bin/bash\n", ""},
		{"empty", "", ""},
	} {
		p := filepath.Join(dir, tc.name)
		write(t, p, tc.script)
		if got := description(p); got != tc.want {
			t.Errorf("%s: description = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := description(filepath.Join(dir, "missing")); got != "" {
		t.Errorf("missing: description = %q", got)
	}
}

// Until they're chosen, base's key has every feature; the first build
// chooses them (without a terminal: all of them) and saves the choice,
// which later builds keep.
func TestChooseFeatures(t *testing.T) {
	b := featureBuilder(t)
	if err := os.Remove(b.Dirs.BaseFeatures()); err != nil {
		t.Fatal(err)
	}
	in, err := b.Inputs()
	if err != nil || !slices.Equal(in.BaseFeatures, []string{"fzf", "omb", "mine"}) {
		t.Fatalf("unchosen: %q, %v", in.BaseFeatures, err)
	}
	before, err := b.Key("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	got, err := state.ReadBaseFeatures(b.Dirs)
	if err != nil || !slices.Equal(got, []string{"fzf", "omb", "mine"}) {
		t.Errorf("saved %q, %v", got, err)
	}
	if after, err := b.Key("base"); err != nil || after != before {
		t.Errorf("base's key went from %s to %s (%v)", before, after, err)
	}
	if err := state.WriteBaseFeatures(b.Dirs, []string{"omb"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Build(t.Context(), "base"); err != nil {
		t.Fatal(err)
	}
	if got, err := state.ReadBaseFeatures(b.Dirs); err != nil || !slices.Equal(got, []string{"omb"}) {
		t.Errorf("a saved choice changed: %q, %v", got, err)
	}
}

// base runs base.sh, then its chosen features' scripts in the order
// chosen, skipping one without a script; a layer runs only its own.
func TestBuildRunsFeatures(t *testing.T) {
	b := featureBuilder(t)
	if err := state.WriteBaseFeatures(b.Dirs, []string{"mine", "nope", "fzf"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Build(t.Context(), "foo"); err != nil {
		t.Fatal(err)
	}
	var got [][]backend.Script
	for _, c := range b.Backend.(*backendtest.Fake).Calls() {
		if c.Method == "BuildImage" {
			got = append(got, c.Args[0].(backend.ImageSpec).Scripts)
		}
	}
	want := [][]backend.Script{
		{
			{Name: "base.sh", Path: filepath.Join(b.Root, "layers", "base.sh")},
			{Name: "mine.sh", Path: filepath.Join(b.Dirs.Features(), "mine.sh")},
			{Name: "fzf.sh", Path: filepath.Join(b.Root, "features", "fzf.sh")},
		},
		{{Name: "foo.sh", Path: filepath.Join(b.Dirs.Layers(), "foo.sh")}},
	}
	if !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("scripts %+v, want %+v", got, want)
	}
}
