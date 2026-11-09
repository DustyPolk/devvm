//go:build !linux && !darwin

package platform

import (
	"context"
	"errors"
)

// ReadClip is unsupported here.
func ReadClip(context.Context, int64) (Clip, error) { return Clip{}, errors.ErrUnsupported }

// CopyText is unsupported here.
func CopyText(context.Context, string) error { return errors.ErrUnsupported }
