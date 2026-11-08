//go:build !linux

package platform

import (
	"errors"
	"os"
	"os/exec"
)

// HasPins reports whether this system can pin paths: only Linux can.
const HasPins = false

// InUserNamespace makes a command start in a namespace of its own: not
// on this system.
func InUserNamespace(*exec.Cmd) error { return errors.ErrUnsupported }

// PrivateMounts makes this mount namespace's mounts private: not on this
// system.
func PrivateMounts() error { return errors.ErrUnsupported }

// OpenRoot opens the folder that pins are beneath: not on this system.
func OpenRoot(string) (*os.File, error) { return nil, errors.ErrUnsupported }

// PinBeneath binds a path over itself: not on this system.
func PinBeneath(*os.File, string, bool) error { return errors.ErrUnsupported }

// Pinned reports whether a path is pinned: never on this system.
func Pinned(*os.File, string, bool) bool { return false }

// ExecUnprivileged replaces devvm with another program: not on this
// system.
func ExecUnprivileged(string, []string, []string) error { return errors.ErrUnsupported }

// WaitExit waits for a process to end: not on this system, where it's
// taken as ended.
func WaitExit(int) (<-chan struct{}, func()) {
	done := make(chan struct{})
	close(done)
	return done, func() {}
}

// WatchFolders watches folders: not on this system.
func WatchFolders([]string) (<-chan struct{}, func(), error) {
	return nil, nil, errors.ErrUnsupported
}

// UsernsLimit is why this host refuses a user namespace pins need: none
// on a system without pins.
func UsernsLimit() string { return "" }
