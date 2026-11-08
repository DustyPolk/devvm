//go:build unix

package safefile

import (
	"errors"
	"io/fs"
	"syscall"
)

// noBlock keeps opening a FIFO from waiting for a writer; noFollow fails on a symbolic link at a
// path's end (ELOOP) rather than opening what it points to.
const (
	noBlock  = syscall.O_NONBLOCK
	noFollow = syscall.O_NOFOLLOW
)

// isLoop reports whether an open failed on a symbolic link noFollow refused.
func isLoop(err error) bool { return errors.Is(err, syscall.ELOOP) }

// links is the number of hard links fi's file has.
func links(fi fs.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Nlink) //nolint:unconvert,nolintlint // Nlink's type differs between systems: uint16 on darwin
	}
	return 1
}
