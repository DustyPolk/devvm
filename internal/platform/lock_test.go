//go:build unix

package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// A file's lock is held by one at a time: another waits for it until its
// context ends, and gets it once it's let go.
func TestLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := LockFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if _, err := LockFile(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("held twice: %v", err)
	}
	got := make(chan error, 1)
	go func() {
		u, err := LockFile(t.Context(), path)
		if err == nil {
			u()
		}
		got <- err
	}()
	time.Sleep(100 * time.Millisecond)
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Errorf("once let go: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("not got once let go")
	}
	if _, err := LockFile(t.Context(), filepath.Join(t.TempDir(), "no", "x.lock")); err == nil {
		t.Error("a lock in a missing folder: no error")
	}
}
