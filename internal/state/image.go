package state

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

// Image is an image's record, images/<key>/record.json. It's written
// last, once the image is built, so an image without one is left over
// from a failed build.
type Image struct {
	Name   string    `json:"name"`   // the image's name: base, docker, docker+foo
	Chain  string    `json:"chain"`  // its build chain: name=key pairs, base first, space-separated
	From   string    `json:"from"`   // the image base started from, if not the default
	Digest string    `json:"digest"` // what base started from: the From image's digest, or the cloud image's
	Libc   string    `json:"libc"`   // the C library: gnu, or musl
	Built  time.Time `json:"built"`  // when its build finished
}

// imageKey is an image key's shape, as image.IsKey has it: state can't
// import image.
var imageKey = regexp.MustCompile(`^[0-9a-f]{12}$`)

// ReadImage reads the record of the image with this key. A missing record
// is an error that matches fs.ErrNotExist.
func ReadImage(d paths.Dirs, key string) (Image, error) {
	if err := checkName("image key", key); err != nil {
		return Image{}, err
	}
	path := d.ImageRecord(key)
	var img Image
	if err := readJSON(path, &img); err != nil {
		return Image{}, err
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
	return writeJSON(d.ImageRecord(key), img)
}

// Images reads every image's record, by key: the folders in the images
// folder named like a key whose record reads. A folder without one is a
// build that didn't finish, or one an older record can't be read in.
func Images(d paths.Dirs) (map[string]Image, error) {
	keys, err := imageKeys(d)
	if err != nil {
		return nil, err
	}
	out := map[string]Image{}
	for _, k := range keys {
		if img, err := ReadImage(d, k); err == nil {
			out[k] = img
		}
	}
	return out, nil
}

// imageKeys are the folders in the images folder named like a key.
func imageKeys(d paths.Dirs) ([]string, error) {
	entries, err := os.ReadDir(d.Images())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if e.IsDir() && imageKey.MatchString(e.Name()) {
			keys = append(keys, e.Name())
		}
	}
	return keys, nil
}

// ImageLock returns the path of the lock the devvm building the image
// with this key holds, images/locks/KEY, making its folder.
func ImageLock(d paths.Dirs, key string) (string, error) {
	if err := checkName("image key", key); err != nil {
		return "", err
	}
	if err := os.MkdirAll(d.ImageLocks(), 0o700); err != nil {
		return "", err
	}
	return filepath.Join(d.ImageLocks(), key), nil
}

// RemoveImage deletes the folder of the image with this key, its record
// with it, if there is one.
func RemoveImage(d paths.Dirs, key string) error {
	if err := checkName("image key", key); err != nil {
		return err
	}
	return os.RemoveAll(d.Image(key))
}

// AnyImage reports whether any image has a record: a folder in the
// images folder named like a key, 12 hex digits, with a record.json.
func AnyImage(d paths.Dirs) (bool, error) {
	keys, err := imageKeys(d)
	if err != nil {
		return false, err
	}
	for _, k := range keys {
		if fi, err := os.Stat(d.ImageRecord(k)); err == nil && fi.Mode().IsRegular() {
			return true, nil
		}
	}
	return false, nil
}

// Rebuild is one image's rebuild counter, which its key hashes: the
// backend that builds it, what its base starts from ("" for the
// backend's own Ubuntu) as given, and its name.
type Rebuild struct {
	Backend string `json:"backend"`
	From    string `json:"from"`
	Name    string `json:"name"`
	Counter string `json:"counter"`
}

// Rebuilds are the rebuild counters, images/rebuilds.json.
type Rebuilds []Rebuild

type rebuildsFile struct {
	Rebuilds Rebuilds `json:"rebuilds"`
}

// ReadRebuilds reads the rebuild counters: none when there's no file.
func ReadRebuilds(d paths.Dirs) (Rebuilds, error) {
	var f rebuildsFile
	err := readJSON(d.Rebuilds(), &f)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return f.Rebuilds, err
}

// Counter is the rebuild counter of the image named name on backend,
// whose base starts from from: "" when it has none.
func (r Rebuilds) Counter(backend, from, name string) string {
	for _, c := range r {
		if c.Backend == backend && c.From == from && c.Name == name {
			return c.Counter
		}
	}
	return ""
}

// BumpRebuild changes the rebuild counter of the image named name on
// backend, whose base starts from from ("" for the default), so its next
// build is a new image there; other backends' and other bases' images
// keep theirs. Its caller holds the counters' lock: it reads them, then
// writes them whole.
func BumpRebuild(d paths.Dirs, backend, from, name string) error {
	if backend == "" || name == "" {
		return errors.New("a rebuild counter needs a backend and an image")
	}
	rs, err := ReadRebuilds(d)
	if err != nil {
		return err
	}
	var r [4]byte
	if _, err := rand.Read(r[:]); err != nil {
		return err
	}
	v := time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(r[:])
	rs = slices.DeleteFunc(rs, func(c Rebuild) bool { return c.Backend == backend && c.From == from && c.Name == name })
	rs = append(rs, Rebuild{Backend: backend, From: from, Name: name, Counter: v})
	slices.SortFunc(rs, func(a, b Rebuild) int {
		return cmp.Or(cmp.Compare(a.Backend, b.Backend), cmp.Compare(a.From, b.From), cmp.Compare(a.Name, b.Name))
	})
	return writeJSON(d.Rebuilds(), rebuildsFile{rs})
}

// isRef accepts an image reference: no spaces, no '='.
func isRef(s string) bool { return s != "" && !strings.ContainsAny(s, " \t=\n\r") }

type fromDigests struct {
	Digests map[string]string `json:"digests"`
}

func readFromDigests(d paths.Dirs) (fromDigests, error) {
	var f fromDigests
	err := readJSON(d.FromDigests(), &f)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if f.Digests == nil {
		f.Digests = map[string]string{}
	}
	return f, err
}

// FromDigest returns the digest last recorded for an image reference in
// the cache, or "" when there is none.
func FromDigest(d paths.Dirs, ref string) (string, error) {
	f, err := readFromDigests(d)
	if err != nil {
		return "", err
	}
	return f.Digests[ref], nil
}

// SetFromDigest records an image reference's digest in the cache,
// replacing the one it had.
func SetFromDigest(d paths.Dirs, ref, digest string) error {
	if !isRef(ref) || strings.ContainsAny(digest, "\n\r") {
		return errors.New("invalid image reference or digest")
	}
	f, err := readFromDigests(d)
	if err != nil {
		return err
	}
	f.Digests[ref] = digest
	return writeJSON(d.FromDigests(), f)
}
