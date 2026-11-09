//go:build unix

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrLinkOnPath is OpenNoLinks's error for a path with a symbolic link
// in it that devvm doesn't follow: one at its end, or one on the way
// that isn't root's.
var ErrLinkOnPath = errors.New("a symbolic link is on the path")

// linkHops is the most links OpenNoLinks follows on one path, Linux's
// own limit, past which it fails with ELOOP.
const linkHops = 40

// OpenNoLinks opens the file at path, an absolute and clean path, for
// reading, following no symbolic link but root's. Each folder on the way
// and the file itself are opened from their parent with O_NOFOLLOW,
// through file descriptors, so there's no moment for a swap between a
// check and the open. A link on the way is followed only when root owns
// it, as the system's are (macOS's /tmp, /var and /etc), because a link
// a VM makes in a mount is the user's on this machine, never root's. Any
// other link on the way, the user's own included, and a link in the
// file's place, fail with ErrLinkOnPath. The file isn't checked further:
// the caller fstats it for a regular file within its cap. Its caller
// closes it.
func OpenNoLinks(path string) (*os.File, error) { return openNoLinks(path, rootOwns) }

// rootOwns reports whether root owns a link, from its fstatat.
func rootOwns(st *unix.Stat_t) bool { return st.Uid == 0 }

// openNoLinks is OpenNoLinks with follow deciding, from a link's fstatat,
// whether a link on the way is followed: tests can't make root's links,
// so they choose their own. A followed link's target replaces it on the
// path: an absolute one starts over from /, a relative one goes on from
// the folder holding the link, and .. goes through openat like any other
// part.
func openNoLinks(path string, follow func(*unix.Stat_t) bool) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("%s: not an absolute, clean path", path)
	}
	parts := pathParts(path)
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s: not a file", path)
	}
	const rootFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC
	fd, err := unix.Open("/", rootFlags, 0)
	if err != nil {
		return nil, err
	}
	hops := 0
	for len(parts) > 0 {
		part, last := parts[0], len(parts) == 1
		parts = parts[1:]
		flags := rootFlags | unix.O_NOFOLLOW
		if last {
			flags = unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		}
		next, err := unix.Openat(fd, part, flags, 0)
		if err == nil {
			_ = unix.Close(fd)
			fd = next
			continue
		}
		st, isLink := linkAt(fd, part)
		switch {
		case !isLink:
			_ = unix.Close(fd)
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		case last || !follow(st):
			_ = unix.Close(fd)
			return nil, fmt.Errorf("%s: %w", path, ErrLinkOnPath)
		}
		if hops++; hops > linkHops {
			_ = unix.Close(fd)
			return nil, &os.PathError{Op: "open", Path: path, Err: unix.ELOOP}
		}
		target, err := readLinkAt(fd, part)
		if err != nil {
			_ = unix.Close(fd)
			return nil, &os.PathError{Op: "readlink", Path: path, Err: err}
		}
		if filepath.IsAbs(target) {
			_ = unix.Close(fd)
			if fd, err = unix.Open("/", rootFlags, 0); err != nil {
				return nil, err
			}
		}
		parts = append(pathParts(target), parts...)
	}
	return os.NewFile(uintptr(fd), path), nil
}

// pathParts splits path into the names openat takes one by one, with
// empty ones and . left out, so a link's target of a/./b or a//b walks
// as a then b.
func pathParts(path string) []string {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	return parts
}

// linkAt reports whether name in the folder dirfd is a symbolic link,
// with its fstatat: what an open with O_NOFOLLOW refused (ELOOP for a
// file, ENOTDIR with O_DIRECTORY on Linux), told apart from a file in a
// folder's place.
func linkAt(dirfd int, name string) (*unix.Stat_t, bool) {
	var st unix.Stat_t
	err := unix.Fstatat(dirfd, name, &st, unix.AT_SYMLINK_NOFOLLOW)
	return &st, err == nil && st.Mode&unix.S_IFMT == unix.S_IFLNK
}

// readLinkAt is the target of the link name in the folder dirfd.
func readLinkAt(dirfd int, name string) (string, error) {
	buf := make([]byte, unix.PathMax)
	n, err := unix.Readlinkat(dirfd, name, buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
