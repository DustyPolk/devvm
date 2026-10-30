//go:build !unix

package platform

import (
	"context"
	"errors"
)

// LockFile is unsupported here.
func LockFile(context.Context, string) (func(), error) {
	return nil, errors.New("locking a file isn't supported on this system")
}
