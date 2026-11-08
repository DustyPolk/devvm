//go:build !linux

package platform

import (
	"errors"
	"time"
)

// RunInNamespace runs a program in a namespace of its own: not on this
// system.
func RunInNamespace(string, []string) error { return errors.ErrUnsupported }

// PinAll pins paths: not on this system.
func PinAll(string, []Pin) error { return errors.ErrUnsupported }

// RunPinned runs a program with paths pinned: not on this system.
func RunPinned(string, []Pin, []string, time.Duration, func(string, string)) error {
	return errors.ErrUnsupported
}

// Probe reports whether this host pins paths: never on this system.
func Probe(func(string) []string, time.Duration) error { return errors.ErrUnsupported }

// ProbePin checks that a path can be pinned: never on this system.
func ProbePin(string) error { return errors.ErrUnsupported }
