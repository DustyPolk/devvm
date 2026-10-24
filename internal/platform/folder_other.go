//go:build !darwin && !linux

package platform

import (
	"context"
	"errors"
)

// FolderID returns what identifies a folder itself: not on this system yet.
func FolderID(string) (string, error) { return "", errors.ErrUnsupported }

// Chflags sets file flags: not on this system.
func Chflags(context.Context, string, string) error { return errors.ErrUnsupported }

// Immutable reports whether a path has the user-immutable flag: not on
// this system.
func Immutable(string) bool { return false }
