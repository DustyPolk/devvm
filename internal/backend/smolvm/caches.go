package smolvm

import (
	"bufio"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/platform"
)

var _ backend.CacheCleaner = (*Smolvm)(nil)

// seedName is a pack's disk in smolvm's cache: v2-<the SHA-256 of the
// pack's .smolmachine>-auto, which smolvm unpacks the first time a
// machine is made from the pack and keeps for the next.
var seedName = regexp.MustCompile(`^v2-([0-9a-f]{64})-auto$`)

// cacheRecent is how long a pack's disk is left alone after it changed:
// a machine may be being made from it.
const cacheRecent = time.Hour

// packCache ties the disks in smolvm's cache to devvm's packs.
type packCache struct{ *Smolvm }

// packsFile lists every pack devvm made, "<key> <sha256>" per line, so a
// pack's disk in smolvm's cache can be tied to devvm's pack after the
// pack is deleted.
func (s packCache) packsFile() string { return filepath.Join(s.Dirs.Smolvm(), "packs") }

// record adds a new pack to packsFile.
func (s packCache) record(key string) error {
	sum, err := image.SHA256File(s.packFile(key))
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.packsFile(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(key + " " + sum + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// packs are devvm's packs by SHA-256: each packsFile names, and each
// pack in its images folder (one made before packsFile was kept is
// hashed now).
func (s packCache) packs() (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(s.packsFile())
	if err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if key, sum, ok := strings.Cut(sc.Text(), " "); ok && image.IsKey(key) {
				out[sum] = key
			}
		}
		_ = f.Close()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	known := map[string]bool{}
	for _, k := range out {
		known[k] = true
	}
	entries, err := os.ReadDir(filepath.Join(s.Dirs.Smolvm(), "images"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !image.IsKey(e.Name()) || known[e.Name()] {
			continue
		}
		if sum, err := image.SHA256File(s.packFile(e.Name())); err == nil {
			out[sum] = e.Name()
		}
	}
	return out, nil
}

// DevvmCache lists the disks smolvm unpacked of devvm's packs (on macOS,
// in smolvm's cache) that no machine of devvm's was made from: a disk
// whose name is the SHA-256 of one of devvm's packs, unchanged for an
// hour. Any other disk there may be of your own packs and is never one;
// so are the folders of machines beside them, and smolvm's other caches.
// On Linux smolvm's state, caches included, is devvm's own folder.
func (s *Smolvm) DevvmCache(ctx context.Context) ([]backend.CacheEntry, error) {
	if s.OS != "darwin" {
		return nil, nil
	}
	seeds, err := packCache{s}.freeSeeds(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]backend.CacheEntry, 0, len(seeds))
	for p, key := range seeds {
		n, _ := platform.DiskUsage(p)
		out = append(out, backend.CacheEntry{What: "the disk of image " + key, Path: p, Size: n})
	}
	return out, nil
}

// DeleteCache deletes pack disks DevvmCache listed that are still free.
func (s *Smolvm) DeleteCache(ctx context.Context, entries []backend.CacheEntry) error {
	seeds, err := packCache{s}.freeSeeds(ctx)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := seeds[e.Path]; !ok {
			continue
		}
		if err := os.RemoveAll(e.Path); err != nil {
			return err
		}
		if err := os.Remove(e.Path + ".lock"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// freeSeeds are the pack disks in smolvm's cache that are devvm's and no
// machine of devvm's was made from, by path, each with its image's key.
func (s packCache) freeSeeds(ctx context.Context) (map[string]string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "smolvm", "vms", packSeeds)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	packs, err := s.packs()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		m := seedName.FindStringSubmatch(e.Name())
		if m == nil || !e.IsDir() {
			continue
		}
		key, ours := packs[m[1]]
		fi, err := e.Info()
		if !ours || err != nil || time.Since(fi.ModTime()) < cacheRecent {
			continue
		}
		users, err := s.imageUsers(ctx, key)
		if err != nil {
			return nil, err
		}
		if len(users) == 0 {
			out[filepath.Join(dir, e.Name())] = key
		}
	}
	return out, nil
}
