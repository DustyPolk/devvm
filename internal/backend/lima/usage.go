package lima

import (
	"context"
	"os"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/platform"
)

var _ backend.UsageReporter = (*Lima)(nil)

// downloadCache is Lima's own download cache, outside its homes: every
// Lima home of the user's shares it, devvm's and their own.
func downloadCache() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lima", "download", "by-url-sha256"), nil
}

// Usage is the disk each VM's folder in devvm's Lima home takes (a clone
// shares its image's blocks on macOS, which du counts for both), and
// Lima's download cache, by entry.
func (l *Lima) Usage(ctx context.Context) (backend.Usage, error) {
	u := backend.Usage{VMs: map[string]int64{}}
	vms, err := l.List(ctx)
	if err != nil {
		return u, err
	}
	for _, in := range vms {
		if n, err := platform.DiskUsage(l.Dirs.LimaVM(in.Name)); err == nil {
			u.VMs[in.Name] = n
		}
	}
	if c, ok := cacheUsage(); ok {
		u.Caches = append(u.Caches, c)
	}
	return u, nil
}

// cacheUsage is Lima's download cache, by entry; ok is false when there's
// none to read.
func cacheUsage() (backend.Cache, bool) {
	dir, err := downloadCache()
	if err != nil {
		return backend.Cache{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return backend.Cache{}, false
	}
	c := backend.Cache{Name: "download cache", Path: filepath.Dir(filepath.Dir(dir)), Count: len(entries)}
	if n, err := platform.DiskUsage(c.Path); err == nil {
		c.Size, c.Sized = n, true
	}
	return c, true
}
