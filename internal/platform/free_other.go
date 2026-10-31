//go:build !darwin && !linux

package platform

import "errors"

// FreeSpace returns the free space on the volume holding path: not on this
// system yet.
func FreeSpace(string) (uint64, error) { return 0, errors.ErrUnsupported }
