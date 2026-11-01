//go:build !unix

package platform

import (
	"context"
	"errors"
)

func noLock() error { return errors.New("locking a file isn't supported on this system") }

// LockFile is unsupported here.
func LockFile(context.Context, string) (func(), error) { return nil, noLock() }

// LockFileShared is unsupported here.
func LockFileShared(context.Context, string) (func(), error) { return nil, noLock() }

// TryLockFile is unsupported here.
func TryLockFile(string) (func(), bool, error) { return nil, false, noLock() }
