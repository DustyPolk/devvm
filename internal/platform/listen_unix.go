//go:build unix

package platform

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
	"time"
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

// ListenLoopback listens on a host loopback address only where the port is
// free on every address (ListenExclusive). A port in use only by
// connections that have ended (TIME_WAIT: nothing answers on it) is taken
// as Go takes any.
func ListenLoopback(ctx context.Context, addr string) (net.Listener, error) {
	l, err := ListenExclusive(ctx, "tcp", addr)
	if err == nil || !errors.Is(err, syscall.EADDRINUSE) {
		return l, err
	}
	d := net.Dialer{Timeout: 300 * time.Millisecond}
	if c, derr := d.DialContext(ctx, "tcp", addr); derr == nil {
		_ = c.Close()
		return nil, err // a program of the host's listens there
	} else if !errors.Is(derr, syscall.ECONNREFUSED) {
		return nil, err
	}
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", addr)
}

// PortFree reports whether a listener of devvm's can have a TCP port on
// the host's loopback, IPv4 and IPv6, sharing it with no program
// (ListenLoopback). A host without IPv6 counts as free there.
func PortFree(ctx context.Context, port int) bool {
	err4, err6 := probeLoopback(ctx, port)
	return err4 == nil && (err6 == nil || !errors.Is(err6, syscall.EADDRINUSE))
}

// PortInUse reports whether a program on the host has a TCP port, on any
// address, so that a listener on the host's loopback with SO_REUSEADDR
// (Go's, Lima's, smolvm's forwards) would share it, and take that
// program's loopback connections: listening on 127.0.0.1 or ::1 without it
// finds the port in use (ListenLoopback). A port that can't be listened on
// for another reason (one below 1024 on Linux) isn't in use.
func PortInUse(ctx context.Context, port int) bool {
	err4, err6 := probeLoopback(ctx, port)
	return errors.Is(err4, syscall.EADDRINUSE) || errors.Is(err6, syscall.EADDRINUSE)
}

// probeLoopback listens on a port on 127.0.0.1 and ::1 (ListenLoopback) a
// moment, and returns each one's error.
func probeLoopback(ctx context.Context, port int) (err4, err6 error) {
	probe := func(host string) error {
		l, err := ListenLoopback(ctx, net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return err
		}
		return l.Close()
	}
	return probe("127.0.0.1"), probe("::1")
}
