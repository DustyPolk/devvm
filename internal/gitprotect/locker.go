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
	// Watches reports whether the protector watches .git too, naming what
	// changed of what a lock covers at each check of a stopped VM: where
	// the locks leave the guest a window (Binds), or where there are none
	// (Watch).
	Watches() bool
	// Lock locks path, a real path in the project folder that's there.
	Lock(ctx context.Context, path string) error
	// Unlock takes the lock off path.
	Unlock(ctx context.Context, path string) error
	// Locked reports whether path itself is locked now.
	Locked(path string) bool
}

// HostLocker is the host's locker: macOS's user-immutable flag (Flags);
// on a host whose VMs' backend pins paths at each start (pins: Linux,
// where the kernel lets devvm), Binds; otherwise Watch.
func HostLocker(pins bool) Locker {
	switch {
	case platform.HasImmutableFlag:
		return Flags{Chflags: platform.Chflags, Immutable: platform.Immutable}
	case pins:
		return Binds{}
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

// Watches reports that the flag needs no watch: it locks each file,
// whatever its names, and the host can't replace it either.
func (Flags) Watches() bool { return false }

// Lock sets the flag on path and everything under it.
func (f Flags) Lock(ctx context.Context, path string) error { return f.Chflags(ctx, "uchg", path) }

// Unlock clears the flag on path and everything under it.
func (f Flags) Unlock(ctx context.Context, path string) error { return f.Chflags(ctx, "nouchg", path) }

// Locked reports whether path has the flag.
func (f Flags) Locked(path string) bool { return f.Immutable != nil && f.Immutable(path) }

// Binds locks on a Linux host: the VM's backend pins what a lock covers
// at each start (Protector.Pins), in QEMU's own namespace, so there's
// nothing to set or take off on the host, and nothing to see there. lock
// still makes a missing hooks folder and config.worktree, to pin.
type Binds struct{}

// Locks reports that pins lock.
func (Binds) Locks() bool { return true }

// Watches reports that pins need the watch too: a file a git command on
// the host renames over a pinned one is the guest's to change until it's
// pinned again, and a write handle it opened then stays writable.
func (Binds) Watches() bool { return true }

// Lock has nothing to set: the start pins.
func (Binds) Lock(context.Context, string) error { return nil }

// Unlock has nothing to take off: pins end with the VM's QEMU.
func (Binds) Unlock(context.Context, string) error { return nil }

// Locked reports every path locked: a hooks folder there at the VM's last
// start was pinned then, as lock made a missing one first, so one the
// guest could fill is a new submodule's, which git_embedded_new names.
func (Binds) Locked(string) bool { return true }

// Watch is the locker of a host without locks: it locks nothing, and the
// protector watches .git instead, naming what changed after the fact.
type Watch struct{}

// Locks reports that it doesn't.
func (Watch) Locks() bool { return false }

// Watches reports that it does: it's all a host without locks has.
func (Watch) Watches() bool { return true }

// Lock locks nothing.
func (Watch) Lock(context.Context, string) error { return nil }

// Unlock has nothing to take off.
func (Watch) Unlock(context.Context, string) error { return nil }

// Locked reports nothing locked.
func (Watch) Locked(string) bool { return false }
