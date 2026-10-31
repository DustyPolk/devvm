//go:build !darwin && !linux

package platform

import "errors"

// freeSpace returns the free space on the volume holding path: not on this
// system yet.
func freeSpace(string) (uint64, error) { return 0, errors.ErrUnsupported }
