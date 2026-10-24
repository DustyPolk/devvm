//go:build !unix

package platform

import "errors"

// Exec replaces devvm with another program: not on this system yet.
func Exec(string, []string, []string) error { return errors.ErrUnsupported }
