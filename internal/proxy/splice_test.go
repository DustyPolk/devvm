package proxy

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

// farEnd is the other end of a tunnel in a test. Each connection's first byte says what it does:
// 'e' reads to the end of what the client sends, answers "got:" and those bytes, and closes; 's'
// sends bytes until a write fails, then says so on gone.
type farEnd struct{ gone chan struct{} }

// serveFarEnd serves ln as a farEnd until it's closed.
func serveFarEnd(t *testing.T, ln net.Listener) *farEnd {
	t.Helper()
	t.Cleanup(func() { _ = ln.Close() })
	f := &farEnd{gone: make(chan struct{}, 16)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *farEnd) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	mode := make([]byte, 1)
	if _, err := io.ReadFull(c, mode); err != nil {
		return
	}
	switch mode[0] {
	case 'e':
		b, _ := io.ReadAll(c)
		_, _ = c.Write(append([]byte("got:"), b...))
	case 's':
		chunk := bytes.Repeat([]byte("x"), 4096)
		_ = c.SetWriteDeadline(time.Now().Add(30 * time.Second))
		for {
			if _, err := c.Write(chunk); err != nil {
				f.gone <- struct{}{}
				return
			}
		}
	}
}

// everyByte is a megabyte holding every byte value, over and over.
func everyByte() []byte {
	b := make([]byte, 1<<20)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// checkTunnel checks what every tunnel does, open asking open for a new one: the client's bytes
// reach the far end as they are, the client's end of sending (a half-close) reaches it too and the
// answer still comes back, and a client that goes away ends the tunnel, so the far end's writes
// fail.
func checkTunnel(t *testing.T, f *farEnd, open func() (net.Conn, io.Reader)) {
	t.Helper()
	c, r := open()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	data := everyByte()
	go func() {
		_, _ = c.Write(append([]byte("e"), data...))
		_ = c.(interface{ CloseWrite() error }).CloseWrite()
	}()
	got, err := io.ReadAll(r)
	_ = c.Close()
	if err != nil || !bytes.Equal(got, append([]byte("got:"), data...)) {
		t.Errorf("after a half-close: %d bytes back, %v; want %d", len(got), err, 4+len(data))
	}

	c, r = open()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	_, _ = c.Write([]byte("s"))
	if _, err := io.ReadFull(r, make([]byte, 64<<10)); err != nil {
		t.Fatalf("the far end's bytes: %v", err)
	}
	_ = c.Close()
	select {
	case <-f.gone:
	case <-time.After(15 * time.Second):
		t.Error("the client went away, and the tunnel goes on")
	}
}

// connectTo opens a CONNECT tunnel through the proxy at addr to hostPort, and returns the
// connection and what reads the tunnel's bytes.
func connectTo(t *testing.T, addr, hostPort string) (net.Conn, io.Reader) {
	t.Helper()
	c, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
	must(t, err)
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", hostPort, hostPort)
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, &http.Request{Method: http.MethodConnect})
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT %s: %d", hostPort, resp.StatusCode)
	}
	return c, r
}

// A tunnel to a host the policy allows.
func TestTunnelSplices(t *testing.T) {
	t.Parallel()
	s, _, _, addr := linkProxy(t)
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	f := serveFarEnd(t, ln)
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	s.egress.pol = writePolicy(t, "127.0.0.1:"+port+"\n")
	checkTunnel(t, f, func() (net.Conn, io.Reader) { return connectTo(t, addr, "127.0.0.1:"+port) })
}

// A tunnel to a host the policy allows passes the far end's bytes on at once: a protocol whose
// server speaks first (SMTP) gets its greeting before the client sends anything, and a client that
// sends fewer than five bytes that aren't TLS gets its answer.
func TestTunnelServerSpeaksFirst(t *testing.T) {
	t.Parallel()
	s, _, _, addr := linkProxy(t)
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = io.WriteString(c, "220 hello\r\n")
		b := make([]byte, 2)
		if _, err := io.ReadFull(c, b); err == nil {
			_, _ = c.Write(append([]byte("got:"), b...))
		}
	}()
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	s.egress.pol = writePolicy(t, "127.0.0.1:"+port+"\n")
	c, r := connectTo(t, addr, "127.0.0.1:"+port)
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	greeting := make([]byte, len("220 hello\r\n"))
	if _, err := io.ReadFull(r, greeting); err != nil || string(greeting) != "220 hello\r\n" {
		t.Fatalf("the greeting before the client spoke: %q, %v", greeting, err)
	}
	_, _ = c.Write([]byte("hi"))
	answer := make([]byte, len("got:hi"))
	if _, err := io.ReadFull(r, answer); err != nil || string(answer) != "got:hi" {
		t.Errorf("two bytes that aren't TLS: %q, %v", answer, err)
	}
}

// A tunnel to one of the VM's links.
func TestLinkTunnelSplices(t *testing.T) {
	t.Parallel()
	s, dir, _, addr := linkProxy(t)
	sock := LinkSocket(dir, linkVM, netip.MustParseAddr("127.77.0.1"), 5432)
	ln, err := new(net.ListenConfig).Listen(t.Context(), "unix", sock)
	must(t, err)
	f := serveFarEnd(t, ln)
	writeLinkList(t, s.links.file, linkEntry(dir, "127.77.0.1", 5432, "db"))
	checkTunnel(t, f, func() (net.Conn, io.Reader) { return connectTo(t, addr, "db:5432") })
}

// splice closes a connection with no writing side of its own to close at the end of what the
// client sent, and returns once the server's bytes end, both connections closed.
func TestSpliceEnds(t *testing.T) {
	t.Parallel()
	client, vm := net.Pipe()
	server, far := net.Pipe()
	done := make(chan struct{})
	go func() { splice(client, server, nil); close(done) }()
	go func() { _, _ = vm.Write([]byte("hi")); _ = vm.Close() }()
	_ = far.SetDeadline(time.Now().Add(10 * time.Second))
	if b, err := io.ReadAll(far); string(b) != "hi" || err != nil {
		t.Errorf("the server read %q, %v", b, err)
	}
	_ = far.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("splice goes on after the server's end")
	}
	if _, err := client.Write([]byte("x")); err == nil {
		t.Error("the client's connection is open")
	}
}

// hijack answers that the tunnel is open, and the bytes the client sent along with its request
// are the connection's first.
func TestHijack(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c, err := hijack(w)
		if err != nil {
			return
		}
		b := make([]byte, 5)
		_, _ = io.ReadFull(c, b)
		_, _ = c.Write(append([]byte("read:"), b...))
		_ = c.Close()
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	c, err := new(net.Dialer).DialContext(t.Context(), "tcp", ln.Addr().String())
	must(t, err)
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = io.WriteString(c, "CONNECT x.test:443 HTTP/1.1\r\nHost: x.test:443\r\n\r\nearly")
	b, _ := io.ReadAll(c)
	if string(b) != "HTTP/1.1 200 Connection established\r\n\r\nread:early" {
		t.Errorf("got %q", b)
	}
}

// The links' addresses are 127.77.0.0/16, as themselves or mapped into IPv6, and LinkAddr gives
// each of them.
func TestIsLinkAddr(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{
		"127.77.0.1": true, "127.77.255.254": true, "127.77.0.0": true, "127.77.255.255": true, "::ffff:127.77.0.1": true,
		"127.0.0.1": false, "127.78.0.0": false, "127.76.255.255": false, "::1": false, "::ffff:127.0.0.1": false,
		"10.77.0.1": false, "0.0.0.0": false, "::": false, "::7f4d:1": false,
	} {
		if got := IsLinkAddr(netip.MustParseAddr(s)); got != want {
			t.Errorf("IsLinkAddr(%s) = %v", s, got)
		}
	}
	if IsLinkAddr(netip.Addr{}) {
		t.Error("IsLinkAddr(the zero Addr) = true")
	}
	for i := range 0x10000 {
		a := LinkAddr(uint16(i))
		if want := fmt.Sprintf("127.77.%d.%d", i>>8, i&0xff); a.String() != want || !IsLinkAddr(a) {
			t.Fatalf("LinkAddr(%d) = %s, IsLinkAddr %v; want %s", i, a, IsLinkAddr(a), want)
		}
	}
}
