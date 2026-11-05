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

// ListenLoopback listens as Go does here.
func ListenLoopback(ctx context.Context, addr string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", addr)
}

// PortInUse can't tell here.
func PortInUse(context.Context, int) bool { return false }
