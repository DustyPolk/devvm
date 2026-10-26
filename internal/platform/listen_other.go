//go:build !unix

package platform

import (
	"context"
	"errors"
	"net"
	"strconv"
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

// PortFree reports whether 127.0.0.1 has the port free, as Go listens.
func PortFree(ctx context.Context, port int) bool {
	l, err := ListenLoopback(ctx, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// PortInUse can't tell here.
func PortInUse(context.Context, int) bool { return false }
