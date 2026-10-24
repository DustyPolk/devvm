// Package platform holds the code that differs between host systems,
// behind build tags: *_unix.go for macOS and Linux, *_darwin.go and
// *_linux.go for one of them, and *_other.go for the rest, which get an
// error until they're supported.
package platform

import "errors"

// ErrNoSecret is KeychainGet's error for an item that isn't there.
var ErrNoSecret = errors.New("no such secret")
