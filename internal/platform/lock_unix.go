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
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // devvm's own lock file
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
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
