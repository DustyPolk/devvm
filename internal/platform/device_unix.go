//go:build unix

package platform

import (
	"io/fs"
	"strconv"
	"syscall"
)

// SameDevice reports whether two files are on the same file system.
func SameDevice(a, b fs.FileInfo) bool {
	sa, oka := a.Sys().(*syscall.Stat_t)
	sb, okb := b.Sys().(*syscall.Stat_t)
	return oka && okb && sa.Dev == sb.Dev
}

// DevIno returns a file's device and inode as "dev:ino", "" when fi has
// none: what the file is, whatever its name.
func DevIno(fi fs.FileInfo) string {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return strconv.FormatUint(uint64(st.Dev), 10) + ":" + strconv.FormatUint(st.Ino, 10) //nolint:gosec,unconvert // Dev's type differs between systems
}
