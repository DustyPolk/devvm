package gitprotect

import (
	"context"

	"github.com/devvmhq/devvm/internal/platform"
)

// A Locker keeps what a lock covers (snapshot.paths) from changing while
// the VM runs, the host's way: the protector decides what to lock and
// when (the VM stopped), and checks each path before and after.
type Locker interface {
	// Locks reports whether it locks at all. One that doesn't (Watch)
	// leaves the protector watching .git instead (watcher).
	Locks() bool
	// Lock locks path, a real path in the project folder that's there.
	Lock(ctx context.Context, path string) error
	// Unlock takes the lock off path.
	Unlock(ctx context.Context, path string) error
	// Locked reports whether path itself is locked now.
	Locked(path string) bool
}

// HostLocker is the host's locker: macOS's user-immutable flag (Flags),
// or, where the host has none, Watch.
func HostLocker() Locker {
	if platform.HasImmutableFlag {
		return Flags{Chflags: platform.Chflags, Immutable: platform.Immutable}
	}
	return Watch{}
}

// Flags locks with macOS's user-immutable flag (chflags uchg), which the
// user sets without root and the VM can't clear.
type Flags struct {
	// Chflags sets file flags on a path and everything under it,
	// following no symlink (platform.Chflags).
	Chflags func(ctx context.Context, flags, path string) error
	// Immutable reports whether a path has the user-immutable flag, not
	// following a symlink (platform.Immutable).
	Immutable func(path string) bool
}

// Locks reports that the flag locks.
func (Flags) Locks() bool { return true }

// Lock sets the flag on path and everything under it.
func (f Flags) Lock(ctx context.Context, path string) error { return f.Chflags(ctx, "uchg", path) }

// Unlock clears the flag on path and everything under it.
func (f Flags) Unlock(ctx context.Context, path string) error { return f.Chflags(ctx, "nouchg", path) }

// Locked reports whether path has the flag.
func (f Flags) Locked(path string) bool { return f.Immutable != nil && f.Immutable(path) }

// Watch is the locker of a host without locks: it locks nothing, and the
// protector watches .git instead, naming what changed after the fact.
type Watch struct{}

// Locks reports that it doesn't.
func (Watch) Locks() bool { return false }

// Lock locks nothing.
func (Watch) Lock(context.Context, string) error { return nil }

// Unlock has nothing to take off.
func (Watch) Unlock(context.Context, string) error { return nil }

// Locked reports nothing locked.
func (Watch) Locked(string) bool { return false }
