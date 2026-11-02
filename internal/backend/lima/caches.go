package lima

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

var _ backend.CacheCleaner = (*Lima)(nil)

// cacheRecent is how long an entry of Lima's cache is left alone after it
// changed: a build may be downloading into it.
const cacheRecent = time.Hour

// DevvmCache lists the entries of Lima's download cache that hold one of
// Ubuntu's cloud images as devvm downloads them for base (its series,
// from its address, a dated release) that no image record of devvm's
// names, so no build of devvm's needs. Lima keeps each download in
// by-url-sha256/<the SHA-256 of its address>, with the address in url:
// an entry whose url doesn't hash to its name is never one, and nor is
// anything else in the cache (your own downloads). Lima copies a VM's
// disk out of the cache when it makes the VM, so no VM needs an entry.
func (l *Lima) DevvmCache(context.Context) ([]backend.CacheEntry, error) {
	dir, err := downloadCache()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	c := l.cache()
	keep, err := c.recordedURLs()
	if err != nil {
		return nil, err
	}
	var out []backend.CacheEntry
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if url, ok := c.devvmEntry(p, keep); ok {
			n, _ := platform.DiskUsage(p)
			out = append(out, backend.CacheEntry{What: "Ubuntu's cloud image " + ubuntu.ReleaseDate(url), Path: p, Size: n})
		}
	}
	return out, nil
}

// DeleteCache deletes entries DevvmCache listed, each one checked again.
func (l *Lima) DeleteCache(_ context.Context, entries []backend.CacheEntry) error {
	dir, err := downloadCache()
	if err != nil {
		return err
	}
	c := l.cache()
	keep, err := c.recordedURLs()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if filepath.Dir(e.Path) != dir {
			continue
		}
		if _, ok := c.devvmEntry(e.Path, keep); ok {
			if err := os.RemoveAll(e.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

// limaCache tells devvm's entries in Lima's download cache apart.
type limaCache struct {
	dirs      paths.Dirs
	ubuntuURL string
}

func (l *Lima) cache() limaCache { return limaCache{dirs: l.Dirs, ubuntuURL: l.UbuntuURL} }

// cloudURL is the address devvm downloads Ubuntu's cloud image from, for
// any dated release.
func (l limaCache) cloudURL() *regexp.Regexp {
	base := cmp.Or(l.ubuntuURL, ubuntu.DefaultURL)
	return regexp.MustCompile(`^` + regexp.QuoteMeta(strings.TrimRight(base, "/")) +
		`/release-[0-9]{8}/ubuntu-` + regexp.QuoteMeta(ubuntu.Version) + `-server-cloudimg-(arm64|amd64)\.img$`)
}

// devvmEntry reports whether a cache entry is devvm's to free, with its
// address: a folder, unchanged for an hour, whose url hashes to its name
// and is devvm's address for a cloud image no record keeps.
func (l limaCache) devvmEntry(path string, keep map[string]bool) (string, bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.IsDir() || time.Since(fi.ModTime()) < cacheRecent {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(path, "url")) //nolint:gosec // Lima's cache, read only
	if err != nil {
		return "", false
	}
	url := strings.TrimSpace(string(data))
	sum := sha256.Sum256([]byte(url))
	if hex.EncodeToString(sum[:]) != filepath.Base(path) || !l.cloudURL().MatchString(url) || keep[url] {
		return "", false
	}
	return url, true
}

// recordedURLs are the cloud images devvm's image records name: a base's
// record keeps its image's address and digest.
func (l limaCache) recordedURLs() (map[string]bool, error) {
	entries, err := os.ReadDir(l.dirs.Images())
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]bool{}, nil
	} else if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	for _, e := range entries {
		if !keyPattern.MatchString(e.Name()) {
			continue
		}
		rec, err := state.ReadImage(l.dirs, e.Name())
		if err != nil {
			continue
		}
		if f := strings.Fields(rec.Digest); len(f) > 0 {
			keep[f[0]] = true
		}
	}
	return keep, nil
}
