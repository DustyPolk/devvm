//go:build unix

package platform

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
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

// LockFileWaiting says it's waiting once, only while another holds the
// lock after a while.
func TestLockFileWaiting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	waits := 0
	waiting := func() { waits++ }
	unlock, err := LockFileWaiting(t.Context(), path, 0, waiting)
	if err != nil || waits != 0 {
		t.Fatalf("a free lock: %v, waits %d", err, waits)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if _, err := LockFileWaiting(ctx, path, 100*time.Millisecond, waiting); !errors.Is(err, context.DeadlineExceeded) || waits != 1 {
		t.Errorf("held: %v, waits %d", err, waits)
	}
	waits = 0
	ctx, cancel = context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if _, err := LockFileWaiting(ctx, path, time.Hour, waiting); !errors.Is(err, context.DeadlineExceeded) || waits != 0 {
		t.Errorf("held, not long: %v, waits %d", err, waits)
	}
	unlock()
}

// A holder may remove the lock file before it lets go: one waiting for
// the removed file takes the lock on the new one, so two never hold it.
func TestLockFileRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := LockFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan func(), 1)
	go func() {
		u, err := LockFile(t.Context(), path)
		if err != nil {
			t.Error(err)
		}
		got <- u
	}()
	time.Sleep(100 * time.Millisecond) // it has opened the file, and waits
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	unlock()
	u := <-got
	if _, ok, err := TryLockFile(path); ok || err != nil {
		t.Errorf("tried while the waiter holds the new file: ok %v, %v", ok, err)
	}
	u()
}

// LockHeld tells an exclusive lock another holds, and takes none.
func TestLockHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	if held, err := LockHeld(path); held || err != nil {
		t.Errorf("no file: %v, %v", held, err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("LockHeld made the file")
	}
	unlock, err := LockFile(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if held, err := LockHeld(path); !held || err != nil {
		t.Errorf("held: %v, %v", held, err)
	}
	unlock()
	if held, err := LockHeld(path); held || err != nil {
		t.Errorf("let go: %v, %v", held, err)
	}
	if unlock, ok, err := TryLockFile(path); !ok || err != nil {
		t.Errorf("after LockHeld: ok %v, %v", ok, err)
	} else {
		unlock()
	}
}

// A shared lock is held by any number at once; TryLockFile doesn't get
// the lock while any holds it, and gets it once all let go, keeping
// shared ones out meanwhile.
func TestLockFileShared(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	one, err := LockFileSharedWaiting(t.Context(), path, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	two, err := LockFileSharedWaiting(t.Context(), path, 0, nil)
	if err != nil {
		t.Fatalf("a second shared lock: %v", err)
	}
	if _, ok, err := TryLockFile(path); ok || err != nil {
		t.Errorf("tried while two held: ok %v, %v", ok, err)
	}
	one()
	if _, ok, err := TryLockFile(path); ok || err != nil {
		t.Errorf("tried while one held: ok %v, %v", ok, err)
	}
	two()
	unlock, ok, err := TryLockFile(path)
	if !ok || err != nil {
		t.Fatalf("tried once let go: ok %v, %v", ok, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if _, err := LockFileSharedWaiting(ctx, path, 0, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("shared while tried: %v", err)
	}
	unlock()
	if _, _, err := TryLockFile(filepath.Join(t.TempDir(), "no", "x.lock")); err == nil {
		t.Error("a lock in a missing folder: no error")
	}
}

// lockHelper is set in the environment of the test binary run as
// TestLockHolder: the lock file it holds.
const lockHelper = "DEVVM_TEST_LOCK_HOLDER"

// TestLockHolder is a process that holds a shared lock until it's killed,
// when run by TestLockFileKilled.
func TestLockHolder(t *testing.T) {
	path := os.Getenv(lockHelper)
	if path == "" {
		t.Skip("run by TestLockFileKilled")
	}
	if _, err := LockFileSharedWaiting(t.Context(), path, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stdout.WriteString("held\n"); err != nil { //nolint:forbidigo // the helper process tells its parent
		t.Fatal(err)
	}
	time.Sleep(time.Minute)
}

// A process killed while holding a shared lock lets go of it.
func TestLockFileKilled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestLockHolder$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), lockHelper+"="+path)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "held\n" {
		t.Fatalf("helper: %q, %v", line, err)
	}
	if _, ok, _ := TryLockFile(path); ok {
		t.Error("got while the helper held it")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	unlock, ok, err := TryLockFile(path)
	if !ok || err != nil {
		t.Fatalf("once the helper was killed: ok %v, %v", ok, err)
	}
	unlock()
}
