//go:build darwin || linux

package platform

import (
	"errors"
	"io/fs"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// FreeSpace returns the free space, in bytes, that an unprivileged user
// can write on the volume holding path: the volume of its nearest folder
// that exists, when path doesn't yet.
func FreeSpace(path string) (uint64, error) {
	path = filepath.Clean(path)
	for {
		var st unix.Statfs_t
		err := unix.Statfs(path, &st)
		if err == nil {
			return uint64(st.Bavail) * uint64(st.Bsize), nil //nolint:gosec,unconvert,nolintlint // their types differ between systems
		}
		up := filepath.Dir(path)
		if !errors.Is(err, fs.ErrNotExist) || up == path {
			return 0, &fs.PathError{Op: "statfs", Path: path, Err: err}
		}
		path = up
	}
}
