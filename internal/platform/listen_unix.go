//go:build unix

package platform

import (
	"context"
	"net"
	"syscall"
)

// ListenExclusive listens on a TCP address without SO_REUSEADDR, which Go
// sets on every listener: with it, macOS and the BSDs let a listener on
// one address (127.0.0.1:5432) take a port another program has on every
// address (*:5432), and with it that program's connections to that
// address. Without it, the port must be free on every address.
func ListenExclusive(ctx context.Context, network, addr string) (net.Listener, error) {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) {
			serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0)
		}); err != nil {
			return err
		}
		return serr
	}}
	return lc.Listen(ctx, network, addr)
}
