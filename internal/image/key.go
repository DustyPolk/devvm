// Package image computes image keys: the hash of everything that goes into
// an image, which names the image and tells when it's stale.
package image

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Base is the image every other one is built on.
const Base = "base"

// keyFormat starts every key's input, so a change to what keys hash
// changes them all.
const keyFormat = "devvm image key 1"

// Input is everything an image's key hashes: what its build is given
// (backend.ImageSpec's name, parent, From and Recipe), the backend, and
// its rebuild counter.
type Input struct {
	// Backend is the backend that builds the image: lima.
	Backend string
	// Image is its name: base, foo+bar.
	Image string
	// Parent is its parent's key; "" for base.
	Parent string
	// From is the container image base starts from, as given, and its
	// digest; "" for Ubuntu's.
	From, FromDigest string
	// Recipe is what its build runs.
	Recipe Recipe
	// Missing is the script of a layer the user deleted, which it has
	// none of: hashed as missing, so the key differs from any contents.
	Missing string
	// Rebuild is its rebuild counter; "" for none.
	Rebuild string
}

// Key is the image's key: 12 hex digits of a SHA-256 over its input, each
// field length-prefixed.
func (in Input) Key() string {
	k := newKeyHash()
	k.field("backend", in.Backend)
	k.field("image", in.Image)
	k.field("parent", in.Parent)
	if in.From != "" {
		k.field("from", in.From)
		k.field("from-digest", in.FromDigest)
	}
	for _, s := range in.Recipe.Origin {
		k.step("origin", s)
	}
	for _, s := range in.Recipe.Steps {
		k.step("step", s)
	}
	if in.Missing != "" {
		k.field("missing", in.Missing)
	}
	if in.Rebuild != "" {
		k.field("rebuild", in.Rebuild)
	}
	return k.sum()
}

// Link is one image in a chain and its key.
type Link struct {
	Name, Key string
}

// Chain is an image and every image under it, base first.
type Chain []Link

// String writes the chain as space-separated name=key pairs, base first:
// "base=1a2b3c4d5e6f node=…".
func (c Chain) String() string {
	parts := make([]string, len(c))
	for i, l := range c {
		parts[i] = l.Name + "=" + l.Key
	}
	return strings.Join(parts, " ")
}

// Key is the key of the chain's top image.
func (c Chain) Key() string {
	if len(c) == 0 {
		return ""
	}
	return c[len(c)-1].Key
}

// ParseChain reads a chain as String writes it. Pairs that aren't
// name=key are skipped.
func ParseChain(s string) Chain {
	var c Chain
	for _, p := range strings.Fields(s) {
		if name, key, ok := strings.Cut(p, "="); ok && name != "" && key != "" {
			c = append(c, Link{name, key})
		}
	}
	return c
}

// LayerName reports whether a layer can have that name: lowercase letters,
// digits and dashes, starting with a letter or digit, and neither base nor
// a shared tool's name. Image names join layers with +.
func LayerName(name string) bool {
	return name != Base && !IsTool(name) && layerName.MatchString(name)
}

// DesktopLayer is the built-in layer with an Xfce desktop, which devvm
// desktop opens.
const DesktopLayer = "desktop"

// DockerLayer is the built-in layer with Docker's engine.
const DockerLayer = "docker"

// NixLayer is the built-in layer with Nix, direnv and nix-direnv.
const NixLayer = "nix"

// Builtins are the layers devvm ships, layers/<name>.sh in its own files,
// sorted: chosen like the user's, whose layers can't take their names.
var Builtins = []string{DesktopLayer, DockerLayer, NixLayer}

// HasLayer reports whether an image's name has the layer.
func HasLayer(name, layer string) bool { return slices.Contains(strings.Split(name, "+"), layer) }

// IsBuiltin reports whether a layer is one devvm ships.
func IsBuiltin(name string) bool { return slices.Contains(Builtins, name) }

var layerName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// FeatureName reports whether a base image feature can have that name:
// lowercase letters, digits and dashes, starting with a letter or digit.
func FeatureName(name string) bool { return layerName.MatchString(name) }

// Parent is the image an image is built on: foo+bar's is foo, foo's is
// base. Base has none ("").
func Parent(name string) string {
	switch i := strings.LastIndex(name, "+"); {
	case name == Base:
		return ""
	case i < 0:
		return Base
	default:
		return name[:i]
	}
}

// Layers splits an image name into its layers, checking each: none for
// base.
func Layers(name string) ([]string, error) {
	if name == Base {
		return nil, nil
	}
	parts := strings.Split(name, "+")
	for _, p := range parts {
		if !LayerName(p) {
			return nil, fmt.Errorf("invalid image name %q", name)
		}
	}
	return parts, nil
}

// keyHash hashes a sequence of fields, each a tag and a value, both
// length-prefixed, so two different sequences never hash the same bytes.
type keyHash struct{ h hash.Hash }

func newKeyHash() keyHash {
	k := keyHash{sha256.New()}
	k.field("format", keyFormat)
	return k
}

func (k keyHash) write(b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	k.h.Write(n[:])
	k.h.Write(b)
}

func (k keyHash) field(tag, value string) { k.bytes(tag, []byte(value)) }

func (k keyHash) bytes(tag string, value []byte) {
	k.write([]byte(tag))
	k.write(value)
}

// own hashes one of devvm's own files, which must be there.
func (k keyHash) own(fsys fs.FS, name string) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("reading devvm's %s: %w", name, err)
	}
	k.bytes("file "+name, data)
	return nil
}

// step hashes a recipe's step, tagged as an origin step or one of its
// steps: its name, its script, how many arguments it has and each one,
// and whether it runs as root.
func (k keyHash) step(tag string, s Step) {
	k.field(tag, s.Name)
	k.bytes("script", s.Data)
	k.field("args", strconv.Itoa(len(s.Args)))
	for _, a := range s.Args {
		k.field("arg", a)
	}
	k.field("root", strconv.FormatBool(s.Root))
}

// keyLen is how many hex digits an image's or a tool image's key has.
const keyLen = 12

// sum is sha12: the first keyLen hex digits of the SHA-256.
func (k keyHash) sum() string { return hex.EncodeToString(k.h.Sum(nil))[:keyLen] }

var keyPattern = regexp.MustCompile(`^[0-9a-f]{` + strconv.Itoa(keyLen) + `}$`)

// IsKey reports whether s is shaped like an image's key, as Key makes it:
// 12 lowercase hex digits.
func IsKey(s string) bool { return keyPattern.MatchString(s) }
