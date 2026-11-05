//go:build unix

package platform

import (
	"net"
	"strconv"
	"testing"
)

// freePort is a port nothing listens on now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0") //nolint:noctx // the test's
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// A port the host has on any address is refused, and in use; one that's
// free, or held only by ended connections, is taken.
func TestListenLoopback(t *testing.T) {
	for name, wildAddr := range map[string]string{"IPv4": "0.0.0.0", "every address": ""} {
		t.Run(name, func(t *testing.T) {
			p := freePort(t)
			addr := "127.0.0.1:" + strconv.Itoa(p)
			wild, err := net.Listen("tcp", net.JoinHostPort(wildAddr, strconv.Itoa(p))) //nolint:noctx // the test's
			if err != nil {
				t.Fatal(err)
			}
			if l, err := ListenLoopback(t.Context(), addr); err == nil {
				_ = l.Close()
				t.Error("took a port the host has on every address")
			}
			if !PortInUse(t.Context(), p) {
				t.Error("not in use")
			}
			// A connection the port's side ends first leaves it in
			// TIME_WAIT.
			c, err := net.Dial("tcp", addr) //nolint:noctx // the test's
			if err != nil {
				t.Fatal(err)
			}
			for { // past the checks' own connections
				s, err := wild.Accept()
				if err != nil {
					t.Fatal(err)
				}
				_ = s.Close()
				if s.RemoteAddr().String() == c.LocalAddr().String() {
					break
				}
			}
			_, _ = c.Read(make([]byte, 1))
			_ = c.Close()
			_ = wild.Close()
			if PortInUse(t.Context(), p) {
				t.Error("in use once its connections ended")
			}
			l, err := ListenLoopback(t.Context(), addr)
			if err != nil {
				t.Fatal(err)
			}
			_ = l.Close()
		})
	}
}

// A port whose connections ListenExclusive's own listener ended first
// (TIME_WAIT) is free again once it's closed: a forwarder started again
// takes it back.
func TestListenExclusiveAgain(t *testing.T) {
	p := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(p)
	l, err := ListenExclusive(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", addr) //nolint:noctx // the test's
	if err != nil {
		t.Fatal(err)
	}
	s, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, _ = c.Read(make([]byte, 1))
	_ = c.Close()
	_ = l.Close()
	if PortInUse(t.Context(), p) {
		t.Error("in use after its connections ended")
	}
	again, err := ListenLoopback(t.Context(), addr)
	if err != nil {
		t.Fatal(err)
	}
	_ = again.Close()
}
