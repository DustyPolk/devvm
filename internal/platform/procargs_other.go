//go:build !darwin && !linux

package platform

import "errors"

// ProcessArgs is unsupported here.
func ProcessArgs(int) ([]string, error) { return nil, errors.ErrUnsupported }
