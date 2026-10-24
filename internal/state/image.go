package state

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/w3cj/devvm/internal/paths"
)

// Image is an image's record, kept per image key. It's written last, once
// the image is built, so an image without one is left over from a failed
// build.
type Image struct {
	Name   string // the image's name: base, node, node+claude
	Chain  string // its build chain: name=key pairs, base first, space-separated
	From   string // the image base started from, if not the default
	Digest string // what base started from: the From image's digest, or the cloud image's
	Libc   string // the C library: gnu, or musl
}

var imageKeys = oneOf("name", "chain", "from", "digest", "libc")

// ReadImage reads the record of the image with this key. A missing record
// is an error that matches fs.ErrNotExist.
func ReadImage(d paths.Dirs, key string) (Image, error) {
	if err := checkName("image key", key); err != nil {
		return Image{}, err
	}
	path := d.ImageState(key)
	k, err := readKV(path, imageKeys)
	if err != nil {
		return Image{}, err
	}
	img := Image{
		Name: k.get("name"), Chain: k.get("chain"), From: k.get("from"),
		Digest: k.get("digest"), Libc: k.get("libc"),
	}
	if img.Name == "" {
		return Image{}, &CorruptError{Path: path, Reason: "no name"}
	}
	return img, nil
}

// WriteImage writes the record of the image with this key.
func WriteImage(d paths.Dirs, key string, img Image) error {
	if err := checkName("image key", key); err != nil {
		return err
	}
	if img.Name == "" {
		return errors.New("an image record needs a name")
	}
	k := newKV()
	for _, p := range [][2]string{
		{"name", img.Name},
		{"chain", img.Chain},
		{"from", img.From},
		{"digest", img.Digest},
		{"libc", img.Libc},
	} {
		if err := checkPair(p[0], p[1], imageKeys); err != nil {
			return err
		}
		k.set(p[0], p[1])
	}
	return writeFile(d.ImageState(key), k.bytes())
}

// RemoveImage deletes the record of the image with this key, if there is
// one.
func RemoveImage(d paths.Dirs, key string) error {
	if err := checkName("image key", key); err != nil {
		return err
	}
	return removeFile(d.ImageState(key))
}

// isRef accepts an image reference as a key: no spaces, no '='.
func isRef(s string) bool { return s != "" && !strings.ContainsAny(s, " \t=") }

func readFromDigests(d paths.Dirs) (kv, error) {
	k, err := readKV(d.FromDigests(), isRef)
	if errors.Is(err, fs.ErrNotExist) {
		return newKV(), nil
	}
	return k, err
}

// FromDigest returns the digest last recorded for an image reference, or ""
// when there is none.
func FromDigest(d paths.Dirs, ref string) (string, error) {
	k, err := readFromDigests(d)
	if err != nil {
		return "", err
	}
	return k.get(ref), nil
}

// SetFromDigest records an image reference's digest, replacing the one it
// had.
func SetFromDigest(d paths.Dirs, ref, digest string) error {
	if err := checkPair(ref, digest, isRef); err != nil {
		return err
	}
	k, err := readFromDigests(d)
	if err != nil {
		return err
	}
	k.set(ref, digest)
	return writeFile(d.FromDigests(), k.bytes())
}
