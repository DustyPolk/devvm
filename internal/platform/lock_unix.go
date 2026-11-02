//go:build unix

package platform

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

// lockTick is how often LockFile tries again for a lock another process
// holds.
const lockTick = 50 * time.Millisecond

// LockFile holds an exclusive lock on path (made if missing, mode 600)
// until unlock is called or the process ends, waiting for another process
// that holds it until ctx ends. Its descriptor isn't inherited (close on
// exec), so a process started meanwhile doesn't keep it. The holder may
// remove the file before it lets go: one that got the lock on the removed
// file tries again on path.
func LockFile(ctx context.Context, path string) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_EX, 0, nil)
}

// LockFileWaiting is LockFile that calls waiting once, when another
// process still holds the lock after it has waited for after.
func LockFileWaiting(ctx context.Context, path string, after time.Duration, waiting func()) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_EX, after, waiting)
}

// LockFileShared holds a shared lock on path, as LockFile does: any
// number of processes hold it at once, and it waits only while one holds
// it exclusively (LockFile, TryLockFile).
func LockFileShared(ctx context.Context, path string) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_SH, 0, nil)
}

// LockFileSharedWaiting is LockFileShared that calls waiting once, when
// another process still holds the lock exclusively after it has waited for
// after.
func LockFileSharedWaiting(ctx context.Context, path string, after time.Duration, waiting func()) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_SH, after, waiting)
}

// TryLockFile is LockFile without waiting: ok is false while another
// holds the lock, shared or exclusive.
func TryLockFile(path string) (unlock func(), ok bool, err error) {
	for {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // devvm's own lock file
		if err != nil {
			return nil, false, err
		}
		got, err := flock(f, syscall.LOCK_EX)
		switch {
		case err != nil:
			_ = f.Close()
			return nil, false, err
		case !got:
			_ = f.Close()
			return nil, false, nil
		case !current(f, path):
			_ = f.Close()
			continue
		}
		return func() { _ = f.Close() }, true, nil
	}
}

// LockHeld reports whether another process holds path's lock
// exclusively; false when there's no such file.
func LockHeld(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // devvm's own lock file
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	got, err := flock(f, syscall.LOCK_SH)
	return !got, err
}

// lockFile takes the lock how says (LOCK_EX or LOCK_SH) on path, trying
// again lockTick apart until ctx ends; waiting, if any, is called once
// after it has waited for after.
func lockFile(ctx context.Context, path string, how int, after time.Duration, waiting func()) (unlock func(), err error) {
	start := time.Now()
	for {
		f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // devvm's own lock file
		if err != nil {
			return nil, err
		}
		for {
			got, err := flock(f, how)
			if err != nil {
				_ = f.Close()
				return nil, err
			}
			if got {
				break
			}
			if waiting != nil && time.Since(start) >= after {
				waiting()
				waiting = nil
			}
			select {
			case <-ctx.Done():
				_ = f.Close()
				return nil, ctx.Err()
			case <-time.After(lockTick):
			}
		}
		if current(f, path) {
			return func() { _ = f.Close() }, nil
		}
		_ = f.Close()
	}
}

// flock tries the lock how says on f once, without waiting: got is false
// while another holds it.
func flock(f *os.File, how int) (got bool, err error) {
	for {
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		}
		return false, err
	}
}

// current reports whether f is still the file at path: the last holder
// may have removed it, or put another there.
func current(f *os.File, path string) bool {
	a, err := f.Stat()
	if err != nil {
		return false
	}
	b, err := os.Stat(path) //nolint:gosec // devvm's own lock file, which f has open
	return err == nil && os.SameFile(a, b)
}
