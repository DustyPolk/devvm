//go:build !darwin && !linux

package platform

import "context"

// ClipboardImage has no reader here: ErrNoClipboardReader, as on a Linux
// machine without one, so callers report it the same way.
func ClipboardImage(context.Context, func(string) string, int64) ([]byte, error) {
	return nil, ErrNoClipboardReader
}
