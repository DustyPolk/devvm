//go:build !unix

package safefile

import "io/fs"

// noBlock and noFollow: none on this system.
const (
	noBlock  = 0
	noFollow = 0
)

// isLoop reports whether an open failed on a symbolic link: never on this system.
func isLoop(error) bool { return false }

// links is the number of hard links fi's file has: one on this system.
func links(fs.FileInfo) uint64 { return 1 }
