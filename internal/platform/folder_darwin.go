package platform

import (
	"context"
	"fmt"
	"os/exec"

	"golang.org/x/sys/unix"
)

// FolderID returns what identifies the folder at path itself, not its
// path: its device and inode. APFS never gives a new folder a deleted
// one's inode number.
func FolderID(path string) (string, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), nil
}

// HasImmutableFlag reports whether this system has a user-immutable flag
// (Chflags, Immutable): macOS's uchg, which the user sets without root.
const HasImmutableFlag = true

// Chflags sets file flags (uchg, nouchg) on path and everything under it,
// following no symlink, path itself included: chflags -R -P.
func Chflags(ctx context.Context, flags, path string) error {
	return exec.CommandContext(ctx, "chflags", "-R", "-P", flags, path).Run() //nolint:gosec // devvm's own arguments
}

// Immutable reports whether path itself (not what a symlink points to) has
// the user-immutable flag (uchg).
func Immutable(path string) bool {
	var st unix.Stat_t
	return unix.Lstat(path, &st) == nil && st.Flags&unix.UF_IMMUTABLE != 0
}
