//go:build unix

package vm

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A FIFO swapped in for default.nix as nixShell reads it never holds it: the VM can make one at
// any moment, and nothing writes to it. (A read that waits stays waiting once the test fails.)
func TestNixShellFIFOSwap(t *testing.T) {
	dir, spare := t.TempDir(), t.TempDir()
	file, fifo := filepath.Join(spare, "file"), filepath.Join(spare, "fifo")
	target := filepath.Join(dir, "default.nix")
	stop, swapped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(swapped)
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.WriteFile(file, []byte("pkgs.mkShell {}\n"), 0o600) != nil || os.Rename(file, target) != nil ||
				syscall.Mkfifo(fifo, 0o600) != nil || os.Rename(fifo, target) != nil {
				return
			}
		}
	}()
	defer func() { close(stop); <-swapped }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for end := time.Now().Add(time.Second); time.Now().Before(end); {
			nixShell(dir)
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("nixShell waits on a FIFO swapped in for default.nix")
	}
}
