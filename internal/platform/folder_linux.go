package platform

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// FolderID returns what identifies the folder at path itself, not its
// path: its device, inode and birth time. ext4 gives a folder made again
// the inode number the deleted one had, so the birth time (where the file
// system keeps one) tells them apart.
func FolderID(path string) (string, error) {
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_INO|unix.STATX_BTIME, &st); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%d:%d", unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino)
	if st.Mask&unix.STATX_BTIME != 0 {
		id += fmt.Sprintf(":%d.%09d", st.Btime.Sec, st.Btime.Nsec)
	}
	return id, nil
}

// Chflags sets file flags: Linux has no unprivileged immutable flag.
func Chflags(context.Context, string, string) error { return errors.ErrUnsupported }
