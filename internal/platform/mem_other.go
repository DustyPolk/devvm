//go:build !darwin && !linux

package platform

import "errors"

// Memory returns the host's memory in bytes: not on this system yet.
func Memory() (uint64, error) { return 0, errors.ErrUnsupported }
