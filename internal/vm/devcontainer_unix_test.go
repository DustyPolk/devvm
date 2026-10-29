//go:build unix

package vm

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/w3cj/devvm/internal/devcontainer"
)

// A FIFO in place of the file is refused at once, not waited on.
func TestDevcontainerFindFIFO(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, devcontainer.RootFile), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (devcontainers{}).find(dir); !errors.Is(err, devcontainer.ErrNotFile) {
		t.Errorf("find = %v", err)
	}
	// One that's a link out of the folder isn't followed.
	if err := os.Remove(filepath.Join(dir, devcontainer.RootFile)); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "x.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, devcontainer.RootFile)); err != nil {
		t.Fatal(err)
	}
	if _, data, err := (devcontainers{}).find(dir); err == nil {
		t.Errorf("read through a link: %q", data)
	}
}
