package platform

import (
	"io/fs"
	"path/filepath"
)

// DiskUsage is the disk space a file, or a folder and everything in it,
// takes, as du counts it: each file's blocks, a file with several links
// once. Links aren't followed. Blocks a copy-on-write clone shares with
// its original count for both.
func DiskUsage(path string) (int64, error) {
	var n int64
	seen := map[string]bool{}
	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if id := DevIno(fi); id != "" {
			if seen[id] {
				return nil
			}
			seen[id] = true
		}
		n += allocated(fi)
		return nil
	})
	return n, err
}
