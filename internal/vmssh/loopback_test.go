package vmssh

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"strconv"
	"testing"
)

// listenAny listens on a port of 127.0.0.1 the system picks.
func listenAny(t *testing.T) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
}

// dialPort connects to 127.0.0.1's port.
func dialPort(t *testing.T, port int) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
}

// freePort is a port nothing on 127.0.0.1 has, just now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := listenAny(t)
	if err != nil {
		t.Fatal(err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return p
}

// listening are the ports f listens on, each once, in order.
func listening(f *LoopbackForward) []int {
	var ps []int
	for _, l := range f.ls {
		if p := l.Addr().(*net.TCPAddr).Port; !slices.Contains(ps, p) {
			ps = append(ps, p)
		}
	}
	return ps
}

// A login's forward: each connection to a port it listens on is carried
// to the same port, until Close; a port a program has is refused.
func TestForwardLoopback(t *testing.T) {
	need, also := freePort(t), freePort(t)
	carry := func(_ context.Context, c net.Conn, port int) {
		_, _ = io.WriteString(c, strconv.Itoa(port))
		_ = c.Close()
	}
	f, err := SSH{}.forwardLoopback(t.Context(), carry, need, also)
	if err != nil {
		t.Fatal(err)
	}
	if got := listening(f); !slices.Equal(got, []int{need, also}) {
		t.Errorf("listening on %v, want %v", got, []int{need, also})
	}
	for _, p := range []int{need, also} {
		c, err := dialPort(t, p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(c)
		_ = c.Close()
		if string(b) != strconv.Itoa(p) {
			t.Errorf("port %d carried to %q", p, b)
		}
	}
	f.Close()
	f.Close()
	if c, err := dialPort(t, need); err == nil {
		_ = c.Close()
		t.Error("the forward still listens after Close")
	}

	taken, err := listenAny(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	port := taken.Addr().(*net.TCPAddr).Port
	if _, err := (SSH{}).forwardLoopback(t.Context(), carry, port); !errors.Is(err, ErrPortTaken) {
		t.Errorf("a taken port: %v, want ErrPortTaken", err)
	}
}

// The forward ends with its context.
func TestForwardLoopbackEndsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	need := freePort(t)
	f, err := SSH{}.forwardLoopback(ctx, func(context.Context, net.Conn, int) {}, need)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	f.wg.Wait()
	if c, err := dialPort(t, need); err == nil {
		_ = c.Close()
		t.Error("the forward still listens after its context ended")
	}
}

// Each connection is a channel of its own to the VM's 127.0.0.1, never a
// master's, with devvm's own connection options.
func TestLoopbackArgs(t *testing.T) {
	o := SSH{VM: "devvm-x", User: "u", Key: "/k", KnownHosts: "/kh", Port: 2222}
	args := o.loopbackArgs(1455)
	for _, want := range [][]string{{"-W", "127.0.0.1:1455"}, {"-o", "ControlPath=none"}, {"-F", "none"}, {"-p", "2222"}} {
		if i := slices.Index(args, want[0]); i < 0 || !slices.Contains(args[i+1:], want[1]) {
			t.Errorf("args %q lack %q", args, want)
		}
	}
	if slices.Contains(args, "-S") || slices.Contains(args, "-M") {
		t.Errorf("args %q use a master", args)
	}
}
