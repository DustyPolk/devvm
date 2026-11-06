//go:build e2e

package e2e

import (
	"context"

	"golang.org/x/sys/unix"
)

// cloneTree makes dst an APFS clone of src, a file or a folder, sharing
// its blocks: it fails where it can't be one (another volume), leaving no
// dst, never copying it whole.
func cloneTree(_ context.Context, src, dst string) error {
	return unix.Clonefile(src, dst, unix.CLONE_NOFOLLOW)
}
