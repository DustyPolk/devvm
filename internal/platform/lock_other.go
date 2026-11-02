//go:build !unix

package platform

import (
	"context"
	"errors"
	"time"
)

func noLock() error { return errors.New("locking a file isn't supported on this system") }

// LockFile is unsupported here.
func LockFile(context.Context, string) (func(), error) { return nil, noLock() }

// LockFileWaiting is unsupported here.
func LockFileWaiting(context.Context, string, time.Duration, func()) (func(), error) {
	return nil, noLock()
}

// LockFileShared is unsupported here.
func LockFileShared(context.Context, string) (func(), error) { return nil, noLock() }

// LockFileSharedWaiting is unsupported here.
func LockFileSharedWaiting(context.Context, string, time.Duration, func()) (func(), error) {
	return nil, noLock()
}

// LockHeld is unsupported here.
func LockHeld(string) (bool, error) { return false, noLock() }

// TryLockFile is unsupported here.
func TryLockFile(string) (func(), bool, error) { return nil, false, noLock() }
