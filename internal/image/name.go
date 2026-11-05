package image

import (
	"fmt"
	"slices"
	"strings"
)

// Name is an image's name: base, or the layers it's built from joined with
// + bottom first, so foo+bar is bar on foo on base. A name the user gives
// is read once, by ParseName; NameOf makes one from layers; a name devvm
// recorded (a VM's state, an image's record, a chain) is converted as it
// is, and Check checks it where a key needs it. Only this package splits
// or joins one.
type Name string

// sep joins an image's layers.
const sep = "+"

// ParseName reads an image's name as the user gives it: its layers bottom
// first, base, empty parts and repeats dropped, so base+foo+bar and
// foo+bar+foo are foo+bar, and base alone when nothing's left. check
// refuses a layer: each is checked in order, and the first error it
// returns is the name's. An empty name is checked as one empty layer.
// A layer check lets through that no layer can have (LayerName) refuses
// the name too.
func ParseName(s string, check func(layer string) error) (Name, error) {
	layers := []string{""}
	if s != "" {
		layers = NameOf(strings.Split(s, sep)).Layers()
	}
	for _, l := range layers {
		if err := check(l); err != nil {
			return "", err
		}
		if !LayerName(l) {
			return "", fmt.Errorf("invalid image name %q", s)
		}
	}
	return NameOf(layers), nil
}

// NameOf is the image with these layers, bottom-up: base when there are
// none. base, empty names and repeats are dropped.
func NameOf(layers []string) Name {
	var parts []string
	for _, p := range layers {
		if p != Base && p != "" && !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return Base
	}
	return Name(strings.Join(parts, sep))
}

// String is the name as messages, records and keys have it.
func (n Name) String() string { return string(n) }

// Layers is the image's layers, bottom-up, as its name has them: none for
// base (or no name).
func (n Name) Layers() []string {
	if n == Base || n == "" {
		return nil
	}
	return strings.Split(string(n), sep)
}

// Check refuses a name that isn't base or layers joined with +, each one
// a layer can have (LayerName).
func (n Name) Check() error {
	for _, p := range n.Layers() {
		if !LayerName(p) {
			return fmt.Errorf("invalid image name %q", string(n))
		}
	}
	if n == "" {
		return fmt.Errorf("invalid image name %q", "")
	}
	return nil
}

// Parent is the image the image is built on: foo+bar's is foo, foo's is
// base. Base has none ("").
func (n Name) Parent() Name {
	switch i := strings.LastIndex(string(n), sep); {
	case n == Base:
		return ""
	case i < 0:
		return Base
	default:
		return n[:i]
	}
}

// Has reports whether the image's name has the layer among its parts:
// base has only base.
func (n Name) Has(layer string) bool {
	return slices.Contains(strings.Split(string(n), sep), layer)
}

// Under is the image's layers' images, bottom-up: foo+bar's are foo and
// foo+bar; base's none.
func (n Name) Under() []Name {
	layers := n.Layers()
	out := make([]Name, len(layers))
	for i := range layers {
		out[i] = Name(strings.Join(layers[:i+1], sep))
	}
	return out
}
