//go:build unix

package gitprotect

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The VM can make any file in .git a FIFO, which nothing writes to: the reads of a git dir's
// files return at once rather than wait on one.
func TestGitFilesFIFO(t *testing.T) {
	g := t.TempDir()
	for _, f := range []string{"commondir", "gitdir"} {
		if err := syscall.Mkfifo(filepath.Join(g, f), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dotGit := filepath.Join(t.TempDir(), ".git")
	if err := syscall.Mkfifo(dotGit, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, read := range map[string]func(){
		"commonOf": func() { commonOf(g) },
		"gitdirOf": func() { gitdirOf(dotGit, filepath.Dir(dotGit)) },
		"gitFile":  func() { _, _ = gitFile(filepath.Join(g, "gitdir")) },
	} {
		done := make(chan struct{})
		go func() { defer close(done); read() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Lets the read go, so the test ends.
			for _, f := range []string{filepath.Join(g, "commondir"), filepath.Join(g, "gitdir"), dotGit} {
				if w, err := os.OpenFile(f, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil { //nolint:gosec // the test's own FIFOs
					_ = w.Close()
				}
			}
			t.Errorf("%s waits on a FIFO", name)
		}
	}
}
