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
// exec), so a process started meanwhile doesn't keep it.
func LockFile(ctx context.Context, path string) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_EX)
}

// LockFileShared holds a shared lock on path, as LockFile does: any
// number of processes hold it at once, and it waits only while one holds
// it exclusively (LockFile, TryLockFile).
func LockFileShared(ctx context.Context, path string) (unlock func(), err error) {
	return lockFile(ctx, path, syscall.LOCK_SH)
}

// TryLockFile is LockFile without waiting: ok is false while another
// holds the lock, shared or exclusive.
func TryLockFile(path string) (unlock func(), ok bool, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // devvm's own lock file
	if err != nil {
		return nil, false, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return func() { _ = f.Close() }, true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			_ = f.Close()
			return nil, false, nil
		}
		_ = f.Close()
		return nil, false, err
	}
}

// lockFile takes the lock how says (LOCK_EX or LOCK_SH) on path, trying
// again lockTick apart until ctx ends.
func lockFile(ctx context.Context, path string, how int) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // devvm's own lock file
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
		if err == nil {
			return func() { _ = f.Close() }, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			_ = f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, ctx.Err()
		case <-time.After(lockTick):
		}
	}
}
