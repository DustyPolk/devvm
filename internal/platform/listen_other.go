//go:build !unix

package platform

import (
	"context"
	"errors"
	"net"
)

// ListenExclusive is unsupported here.
func ListenExclusive(context.Context, string, string) (net.Listener, error) {
	return nil, errors.New("listening without address reuse isn't supported on this system")
}
