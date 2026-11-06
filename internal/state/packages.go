package state

import (
	"errors"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/paths"
)

// PackageSet is the packages a base recorded at its first build,
// packages/sets/<key>.json: every package its build downloaded, each as
// apt's package lists had it then, which apt had checked against the
// archive's signature. Every later build of that key installs these from
// devvm's cache (packages/<sha256>.deb) and nothing else, so a key's
// packages never change. It's written once, before the image's record.
type PackageSet struct {
	Recorded time.Time `json:"recorded"`
	Packages []Package `json:"packages"`
}

// Package is one package of a set.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
	SHA256  string `json:"sha256"` // its file's, which names it in the cache
	Size    int64  `json:"size"`
	// Entry is its entry in apt's package lists, which a replay's own
	// list is made of.
	Entry string `json:"entry"`
}

var (
	pkgName    = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*$`)
	pkgVersion = regexp.MustCompile(`^[A-Za-z0-9.+~:-]+$`)
	pkgArch    = regexp.MustCompile(`^[a-z0-9-]+$`)
	pkgSHA256  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Valid reports whether p's fields have their shapes: its name, version
// and architecture as Debian's, its SHA-256 64 hex digits, a size, and an
// entry with no blank line, or one of only spaces, which would end it.
func (p Package) Valid() bool {
	if !pkgName.MatchString(p.Name) || !pkgVersion.MatchString(p.Version) || !pkgArch.MatchString(p.Arch) ||
		!pkgSHA256.MatchString(p.SHA256) || p.Size <= 0 || p.Entry == "" || strings.ContainsAny(p.Entry, "\r\x00") {
		return false
	}
	for line := range strings.Lines(strings.TrimSuffix(p.Entry, "\n")) {
		if strings.TrimSpace(line) == "" {
			return false
		}
	}
	return true
}

// ReadPackageSet reads the set the base with key recorded. None is an
// error that matches fs.ErrNotExist; one with a package that isn't Valid,
// or twice, is a CorruptError.
func ReadPackageSet(d paths.Dirs, key string) (PackageSet, error) {
	if err := checkName("image key", key); err != nil {
		return PackageSet{}, err
	}
	var s PackageSet
	if err := readJSON(d.PackageSet(key), &s); err != nil {
		return PackageSet{}, err
	}
	if !s.valid() {
		return PackageSet{}, &CorruptError{Path: d.PackageSet(key), Reason: "not a package set"}
	}
	return s, nil
}

func (s PackageSet) valid() bool {
	seen := map[string]bool{}
	for _, p := range s.Packages {
		if !p.Valid() || seen[p.SHA256] {
			return false
		}
		seen[p.SHA256] = true
	}
	return true
}

// WritePackageSet writes the set the base with key recorded.
func WritePackageSet(d paths.Dirs, key string, s PackageSet) error {
	if err := checkName("image key", key); err != nil {
		return err
	}
	if !s.valid() {
		return errors.New("a package set with a package that isn't one")
	}
	return writeJSON(d.PackageSet(key), s)
}

// PackageSetKeys lists the keys that have a package set, sorted.
func PackageSetKeys(d paths.Dirs) ([]string, error) {
	entries, err := os.ReadDir(d.PackageSets())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, e := range entries {
		if k, ok := strings.CutSuffix(e.Name(), ".json"); ok && imageKey.MatchString(k) && e.Type().IsRegular() {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys, nil
}

// RemovePackageSet deletes the set the base with key recorded; none is
// not an error.
func RemovePackageSet(d paths.Dirs, key string) error {
	if err := checkName("image key", key); err != nil {
		return err
	}
	if err := os.Remove(d.PackageSet(key)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
