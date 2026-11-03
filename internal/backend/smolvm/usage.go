package smolvm

import (
	"context"
	"os"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/platform"
)

var _ backend.UsageReporter = (*Smolvm)(nil)

// caches are smolvm's caches on macOS, under the user's cache folder,
// which every machine of the user's shares with devvm's: what smolvm
// unpacked of each pack a machine was made from (a disk per pack, named
// by the SHA-256 of its .smolmachine), of each container image, and its
// downloads. Its machines' own folders are beside them, in vms/, and
// aren't caches.
func caches() []struct{ name, path string } {
	return []struct{ name, path string }{
		{"pack disks", "smolvm/vms/" + packSeeds},
		{"image disks", "smolvm/image-seeds"},
		{"image archives", "smolvm-image-archives"},
		{"registry", "smolvm-registry"},
	}
}

// packSeeds is the folder of smolvm's cache in which it keeps the disk it
// unpacked of each pack, as v2-<the pack's SHA-256>-auto.
const packSeeds = "_pack-storage-seeds"

// Usage is the disk each of devvm's machines takes (its folder, as
// smolvm machine data-dir says), and on macOS smolvm's caches. On Linux
// smolvm's state is devvm's own, in its data folder, and has no caches
// shown apart.
func (s *Smolvm) Usage(ctx context.Context) (backend.Usage, error) {
	u := backend.Usage{VMs: map[string]int64{}}
	vms, err := s.List(ctx)
	if err != nil {
		return u, err
	}
	for _, in := range vms {
		if dir := s.dataDir(ctx, in.Name); dir != "" {
			if n, err := platform.DiskUsage(dir); err == nil {
				u.VMs[in.Name] = n
			}
		}
	}
	if s.OS == "darwin" {
		u.Caches = cacheUsage(s.Dirs.UserCache)
	}
	return u, nil
}

// cacheUsage is smolvm's caches on macOS, each by entry, in the user's
// cache folder (paths.Dirs.UserCache).
func cacheUsage(cache string) []backend.Cache {
	if cache == "" {
		return nil
	}
	var out []backend.Cache
	for _, c := range caches() {
		dir := filepath.Join(cache, c.path)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		cache := backend.Cache{Name: c.name, Path: dir}
		for _, e := range entries {
			if e.IsDir() && e.Name()[0] != '.' {
				cache.Count++
			}
		}
		if n, err := platform.DiskUsage(dir); err == nil {
			cache.Size, cache.Sized = n, true
		}
		out = append(out, cache)
	}
	return out
}
