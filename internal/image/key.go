// Package image computes image keys: the hash of everything that goes into
// an image, which names the image and tells when it's stale.
package image

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/paths"
)

// Base is the image every other one is built on.
const Base = "base"

// keyFormat starts every key's input, so a change to what keys hash
// changes them all.
const keyFormat = "devvm image key 1"

// Inputs is everything an image key hashes apart from the image's name.
// The files are read when a key is computed.
type Inputs struct {
	// Backend is the backend that builds the image: lima, smolvm, docker.
	Backend string
	// BackendFiles are the backend's own files that go into base
	// (backend.KeyFiles), by name. Nil for none.
	BackendFiles map[string][]byte
	// Assets holds devvm's own files (assets.FS): base.yaml,
	// layers/base.sh, the built-in layers, features/ and oci/. Files are
	// hashed by their names in it.
	Assets fs.FS
	// Layers holds the user's layers, <name>.sh (paths.Dirs.Layers). Nil
	// when there are none. A built-in layer's name is never read here.
	Layers fs.FS
	// Features holds the user's features, <name>.sh (paths.Dirs.Features).
	// Nil when there are none. A built-in feature of the same name wins.
	Features fs.FS
	// BaseFeatures are the features chosen for base, in order.
	BaseFeatures []string
	// From is the image base starts from; empty for Ubuntu's cloud image.
	From string
	// FromDigest is From's digest, as recorded when it was looked up.
	FromDigest string
	// Rebuilds holds the rebuild counters (paths.Dirs.Rebuilds), named by
	// paths.RebuildName. Nil when there are none.
	Rebuilds fs.FS
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

// Outdated returns the images in chain, base first, whose key has changed
// since it was made: an edited script, other features, a rebuild.
func (in Inputs) Outdated(chain Chain) ([]string, error) {
	var changed []string
	for _, l := range chain {
		key, err := in.Key(l.Name)
		if err != nil {
			return nil, err
		}
		if key != l.Key {
			changed = append(changed, l.Name)
		}
	}
	return changed, nil
}

// LayerName reports whether a layer can have that name: lowercase letters,
// digits and dashes, starting with a letter or digit, and neither base, a
// shared tool's name nor a word devvm keeps for its commands. Image names
// join layers with +.
func LayerName(name string) bool {
	return name != Base && !IsTool(name) && !IsReserved(name) && layerName.MatchString(name)
}

// DockerLayer is the built-in layer with Docker's engine.
const DockerLayer = "docker"

// NixLayer is the built-in layer with Nix, direnv and nix-direnv.
const NixLayer = "nix"

// Builtins are the layers devvm ships, layers/<name>.sh in its own files:
// chosen like the user's, whose layers can't take their names.
var Builtins = []string{DockerLayer, NixLayer}

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

// layers splits an image name into its layers, checking each.
func layers(name string) ([]string, error) {
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

// Key is the image's key: 12 hex digits of a SHA-256 over its inputs and
// its parent's key.
func (in Inputs) Key(name string) (string, error) {
	c, err := in.Chain(name)
	if err != nil {
		return "", err
	}
	return c.Key(), nil
}

// Chain is the image and every image it's built on, with their keys,
// base first. A changed key anywhere in it changes every key above.
func (in Inputs) Chain(name string) (Chain, error) {
	parts, err := layers(name)
	if err != nil {
		return nil, err
	}
	if in.Backend == "" {
		return nil, errors.New("image keys need a backend")
	}
	key, err := in.baseKey()
	if err != nil {
		return nil, err
	}
	chain := Chain{{Base, key}}
	for i, layer := range parts {
		img := strings.Join(parts[:i+1], "+")
		if key, err = in.layerKey(img, layer, key); err != nil {
			return nil, err
		}
		chain = append(chain, Link{img, key})
	}
	return chain, nil
}

// baseKey hashes base: the backend and its own files, base.yaml,
// layers/base.sh, the image it starts from with its digest and the scripts
// that convert it, the chosen features and its rebuild counter.
func (in Inputs) baseKey() (string, error) {
	k := newKeyHash()
	k.field("backend", in.Backend)
	for _, name := range slices.Sorted(maps.Keys(in.BackendFiles)) {
		k.bytes("backend-file "+name, in.BackendFiles[name])
	}
	for _, f := range []string{"base.yaml", "layers/base.sh"} {
		if err := k.own(in.Assets, f); err != nil {
			return "", err
		}
	}
	if in.From != "" {
		k.field("from", in.From)
		k.field("from-digest", in.FromDigest)
		for _, f := range []string{"oci/build.sh", "oci/bootify.sh", "oci/kernel.sh"} {
			if err := k.own(in.Assets, f); err != nil {
				return "", err
			}
		}
	}
	for _, f := range in.BaseFeatures {
		if !FeatureName(f) {
			return "", fmt.Errorf("invalid feature name %q", f)
		}
		k.field("feature", f)
		builtin, err := k.optional("builtin-feature", in.Assets, "features/"+f+".sh")
		if err != nil {
			return "", err
		}
		if !builtin {
			if _, err := k.optional("user-feature", in.Features, f+".sh"); err != nil {
				return "", err
			}
		}
	}
	if err := k.rebuild(in, Base); err != nil {
		return "", err
	}
	return k.sum(), nil
}

// layerKey hashes a layer: its parent's key, its name, its script (devvm's
// for a built-in layer, else the user's) and its rebuild counter.
func (in Inputs) layerKey(img, layer, parent string) (string, error) {
	k := newKeyHash()
	k.field("parent", parent)
	k.field("layer", layer)
	if IsBuiltin(layer) {
		if err := k.own(in.Assets, "layers/"+layer+".sh"); err != nil {
			return "", err
		}
	} else if _, err := k.optional("user-layer", in.Layers, layer+".sh"); err != nil {
		return "", err
	}
	if err := k.rebuild(in, img); err != nil {
		return "", err
	}
	return k.sum(), nil
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

// optional hashes a file that may be missing (a layer or feature the user
// deleted): that's hashed as missing, so the key differs from any
// contents. It reports whether the file is there.
func (k keyHash) optional(tag string, fsys fs.FS, name string) (bool, error) {
	if fsys != nil {
		data, err := fs.ReadFile(fsys, name)
		if err == nil {
			k.bytes(tag+" "+name, data)
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	k.field("missing", tag+" "+name)
	return false, nil
}

// rebuild hashes the image's rebuild counter, when it has one.
func (k keyHash) rebuild(in Inputs, img string) error {
	if in.Rebuilds == nil {
		return nil
	}
	data, err := fs.ReadFile(in.Rebuilds, paths.RebuildName(img, in.From))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	k.bytes("rebuild", data)
	return nil
}

// sum is sha12: the first 12 hex digits of the SHA-256.
func (k keyHash) sum() string { return hex.EncodeToString(k.h.Sum(nil))[:12] }
