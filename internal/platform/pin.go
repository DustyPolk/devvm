package platform

import "errors"

// A Pin is a path below a folder that a namespace pins (PinAll): relative
// to it, and read-only, or fixed in place but writable.
type Pin struct {
	Path     string
	ReadOnly bool
}

// ErrLinked is PinBeneath's refusal of a file with another name: a bind
// covers one name, so another, in the project folder, would stay writable
// (macOS's flag locks the file itself, whatever its names).
var ErrLinked = errors.New("it has another name (a hard link)")

// What RunPinned reports of a pin while it keeps them.
const (
	PinAgain  = "pinned-again" // it lost its pin (a file renamed over it), and was pinned again
	PinLinked = "linked"       // it has another name now, so it can't be pinned
	PinLost   = "lost"         // it lost its pin and can't be pinned again
)
