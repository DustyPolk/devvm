//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
)

// cloneTree makes dst a reflink of src, a file or a folder, sharing its
// blocks: it fails where the file system can't (tmpfs, ext4), leaving no
// dst, never copying it whole.
func cloneTree(ctx context.Context, src, dst string) error {
	if err := exec.CommandContext(ctx, "cp", "-a", "--reflink=always", src, dst).Run(); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}
