//go:build !darwin && !linux

package platform

import "errors"

// memory returns the host's memory in bytes: not on this system yet.
func memory() (uint64, error) { return 0, errors.ErrUnsupported }
