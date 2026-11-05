package image

import (
	"errors"
	"slices"
	"testing"
)

// A name as the user gives it: layers as given, each once; base and empty
// parts dropped, and base alone for nothing else. Each layer is checked
// in order, the first refusal the name's.
func TestParseName(t *testing.T) {
	var checked []string
	ok := func(l string) error { checked = append(checked, l); return nil }
	for in, want := range map[string]Name{
		"base":         "base",
		"+":            "base",
		"base+base":    "base",
		"base+foo":     "foo",
		"foo+base+bar": "foo+bar",
		"foo+bar":      "foo+bar",
		"bar+foo":      "bar+foo",
		"foo+bar+foo":  "foo+bar",
		"foo++bar+":    "foo+bar",
	} {
		checked = nil
		got, err := ParseName(in, ok)
		if err != nil || got != want {
			t.Errorf("ParseName(%q) = %q, %v; want %q", in, got, err, want)
		}
		if !slices.Equal(checked, want.Layers()) {
			t.Errorf("ParseName(%q) checked %q", in, checked)
		}
	}
	// An empty name is one empty layer: check refuses it, or the name is
	// invalid.
	checked = nil
	if _, err := ParseName("", ok); err == nil || err.Error() != `invalid image name ""` || !slices.Equal(checked, []string{""}) {
		t.Errorf(`ParseName("") = %v, checked %q`, err, checked)
	}
	// What check lets through that no layer can have.
	for _, in := range []string{"Foo", "foo+../x", "foo/bar", "-x", "node", "foo+claude"} {
		if _, err := ParseName(in, ok); err == nil || err.Error() != `invalid image name "`+in+`"` {
			t.Errorf("ParseName(%q) = %v", in, err)
		}
	}
	// The first refusal, bottom first.
	refuse := errors.New("refused")
	checked = nil
	if _, err := ParseName("foo+bar+baz", func(l string) error {
		checked = append(checked, l)
		if l == "bar" {
			return refuse
		}
		return nil
	}); !errors.Is(err, refuse) || !slices.Equal(checked, []string{"foo", "bar"}) {
		t.Errorf("ParseName refused = %v, checked %q", err, checked)
	}
}

// NameOf makes a name from layers, and Layers gives them back.
func TestNameOfAndLayers(t *testing.T) {
	for _, c := range []struct {
		layers []string
		want   Name
	}{
		{nil, Base},
		{[]string{}, Base},
		{[]string{"base"}, Base},
		{[]string{""}, Base},
		{[]string{"foo"}, "foo"},
		{[]string{"foo", "bar"}, "foo+bar"},
		{[]string{"base", "foo", "", "foo", "bar"}, "foo+bar"},
	} {
		got := NameOf(c.layers)
		if got != c.want {
			t.Errorf("NameOf(%q) = %q, want %q", c.layers, got, c.want)
		}
		// The round trip: a name's layers make it again.
		if again := NameOf(got.Layers()); again != got {
			t.Errorf("NameOf(%q.Layers()) = %q", got, again)
		}
	}
	for n, want := range map[Name][]string{"base": nil, "": nil, "foo": {"foo"}, "foo+bar": {"foo", "bar"}, "base+foo": {"base", "foo"}} {
		if got := n.Layers(); !slices.Equal(got, want) {
			t.Errorf("%q.Layers() = %q, want %q", n, got, want)
		}
	}
	if got := Name("foo+bar").String(); got != "foo+bar" {
		t.Errorf("String() = %q", got)
	}
}

// Check refuses a recorded name that isn't base or layers'.
func TestNameCheck(t *testing.T) {
	for _, n := range []Name{"base", "foo", "foo+bar", "foo+foo", "docker+nix"} {
		if err := n.Check(); err != nil {
			t.Errorf("%q.Check(): %v", n, err)
		}
	}
	for _, n := range []Name{"", "Foo", "foo+", "+foo", "base+foo", "foo+base", "foo/bar", "../x", "-x", "node", "foo+claude"} {
		if err := n.Check(); err == nil || err.Error() != `invalid image name "`+string(n)+`"` {
			t.Errorf("%q.Check() = %v", n, err)
		}
	}
}

func TestNameParent(t *testing.T) {
	for n, want := range map[Name]Name{"base": "", "foo": "base", "foo+bar": "foo", "a+b+c": "a+b"} {
		if got := n.Parent(); got != want {
			t.Errorf("%q.Parent() = %q, want %q", n, got, want)
		}
	}
}

func TestNameUnder(t *testing.T) {
	for n, want := range map[Name][]Name{"base": {}, "foo": {"foo"}, "a+b+c": {"a", "a+b", "a+b+c"}} {
		if got := n.Under(); !slices.Equal(got, want) {
			t.Errorf("%q.Under() = %q, want %q", n, got, want)
		}
	}
}

// Has finds a layer among an image name's parts: base has only base.
func TestNameHas(t *testing.T) {
	for _, c := range []struct {
		name  Name
		layer string
		want  bool
	}{
		{"base", "base", true},
		{"base", "nix", false},
		{"foo", "base", false},
		{"nix", "nix", true},
		{"foo+nix", "nix", true},
		{"nix+foo", "nix", true},
		{"nixos", "nix", false},
		{"foo+nixos", "nix", false},
		{"docker+nix+foo", "docker", true},
	} {
		if got := c.name.Has(c.layer); got != c.want {
			t.Errorf("%q.Has(%q) = %v", c.name, c.layer, got)
		}
	}
}
