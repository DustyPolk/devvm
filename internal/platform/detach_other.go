//go:build !unix

package platform

import (
	"errors"
	"os"
)

// StartDetached is unsupported here.
func StartDetached(string, []string, *os.File) (int, error) {
	return 0, errors.New("starting a detached process isn't supported on this system")
}
