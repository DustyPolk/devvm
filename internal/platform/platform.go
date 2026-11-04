// Package platform holds the code that differs between host systems,
// behind build tags: *_unix.go for macOS and Linux, *_darwin.go and
// *_linux.go for one of them, and *_other.go for the rest, which get an
// error until they're supported.
package platform

import (
	"errors"
	"os"
	"strings"
)

// ErrNoSecret is KeychainGet's error for an item that isn't there.
var ErrNoSecret = errors.New("no such secret")

// FolderDevice returns the device in a FolderID, "" when id has none.
func FolderDevice(id string) string {
	dev, _, ok := strings.Cut(id, ":")
	if !ok {
		return ""
	}
	return dev
}

// DeviceOf returns the device of what's at path, following a symlink, as
// FolderID writes it.
func DeviceOf(path string) (string, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	dev := FolderDevice(DevIno(fi))
	if dev == "" {
		return "", errors.ErrUnsupported
	}
	return dev, nil
}
