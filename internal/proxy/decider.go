package proxy

import (
	"bytes"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"
)

// The decider (-decider): for a backend that asks the host about each connection a VM opens
// (smolvm's mediated egress, protocol version 2), so the VM reaches nothing but this proxy. The
// backend connects once per connection the VM opens and writes a prelude: "SMOLMEG2", the 32-byte
// token it was started with, a 16-byte machine ID, a 16-byte parent ID, the IP family (4 or 6), the
// destination port, its address, and a two-byte length followed by that many of the connection's
// first bytes (at most 8 KiB). Nothing past the token is read without it. The answer is one byte: 1 refuses the connection, 2 takes it over, after which the
// same connection carries it (the first bytes aren't sent again). Only a connection to -decider-to,
// the proxy's address in the VM, is taken over, and the proxy then serves it as any other: its
// password is still needed. Everything else is refused and logged; so is a prelude without the
// token, which gets no answer at all. A decider that's gone refuses everything (the backend
// fails closed).

var (
	deciderAddr      = ""
	deciderTo        = ""
	deciderTokenFile = ""
)

const (
	deciderMagic    = "SMOLMEG2"
	deciderRefuse   = 1
	deciderTakeOver = 2
	// preludeTimeout bounds how long a prelude may take to arrive.
	preludeTimeout = 10 * time.Second
)

// readDeciderToken reads the token file: 64 hex digits, not all zero.
func readDeciderToken(path string) ([]byte, error) {
	b, err := os.ReadFile(path) //nolint:gosec // devvm names the file
	if err != nil {
		return nil, err
	}
	tok, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(tok) != 32 || bytes.Equal(tok, make([]byte, 32)) {
		return nil, fmt.Errorf("%s: not 64 hex digits", path)
	}
	return tok, nil
}

// prelude is what the backend says about one connection the VM opens.
type prelude struct {
	token []byte
	dest  netip.AddrPort
	first []byte
}

var (
	errPrelude = errors.New("not a decider prelude")
	errToken   = errors.New("a prelude without the token")
)

// maxFirst is the most first bytes smolvm sends: 8 KiB.
const maxFirst = 8 << 10

// readPrelude reads a prelude from r, which must carry token: nothing past the token is read
// without it.
func readPrelude(r io.Reader, token []byte) (prelude, error) {
	var p prelude
	head := make([]byte, len(deciderMagic)+32)
	if _, err := io.ReadFull(r, head); err != nil {
		return p, err
	}
	if string(head[:len(deciderMagic)]) != deciderMagic {
		return p, errPrelude
	}
	p.token = head[len(deciderMagic):]
	if subtle.ConstantTimeCompare(p.token, token) != 1 {
		return p, errToken
	}
	rest := make([]byte, 16+16+1+2)
	if _, err := io.ReadFull(r, rest); err != nil {
		return p, err
	}
	rest = rest[16+16:]
	family, port := rest[0], binary.BigEndian.Uint16(rest[1:3])
	var addr netip.Addr
	switch family {
	case 4:
		var a [4]byte
		if _, err := io.ReadFull(r, a[:]); err != nil {
			return p, err
		}
		addr = netip.AddrFrom4(a)
	case 6:
		var a [16]byte
		if _, err := io.ReadFull(r, a[:]); err != nil {
			return p, err
		}
		addr = netip.AddrFrom16(a)
	default:
		return p, errPrelude
	}
	p.dest = netip.AddrPortFrom(addr.Unmap(), port)
	var n [2]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return p, err
	}
	size := binary.BigEndian.Uint16(n[:])
	if size > maxFirst {
		return p, errPrelude
	}
	p.first = make([]byte, size)
	if _, err := io.ReadFull(r, p.first); err != nil {
		return p, err
	}
	return p, nil
}

// handover is a listener of the connections the decider took over, which the proxy's server
// serves; closing it closes the decider's own listener too.
type handover struct {
	ln    net.Listener
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newHandover(ln net.Listener) *handover {
	return &handover{ln: ln, conns: make(chan net.Conn), done: make(chan struct{})}
}

func (h *handover) Accept() (net.Conn, error) {
	select {
	case c := <-h.conns:
		return c, nil
	case <-h.done:
		return nil, net.ErrClosed
	}
}

func (h *handover) Close() error {
	h.once.Do(func() { close(h.done); _ = h.ln.Close() })
	return nil
}

func (h *handover) Addr() net.Addr { return h.ln.Addr() }

// firstConn is a connection whose first bytes were read already: Read gives them first.
type firstConn struct {
	net.Conn
	r io.Reader
}

func (c *firstConn) Read(b []byte) (int, error) { return c.r.Read(b) }

// serveDecider answers the backend's connections on h's listener until it's closed: those to the
// proxy (to) with the token are handed over to the proxy's server through h.
func serveDecider(h *handover, token []byte, to netip.AddrPort) {
	for {
		c, err := h.ln.Accept()
		if err != nil {
			return
		}
		go decide(c, h, token, to)
	}
}

func decide(c net.Conn, h *handover, token []byte, to netip.AddrPort) {
	_ = c.SetDeadline(time.Now().Add(preludeTimeout))
	p, err := readPrelude(c, token)
	if err != nil {
		logf("decider: %v", err)
		_ = c.Close()
		return
	}
	if p.dest != to {
		_, _ = c.Write([]byte{deciderRefuse})
		_ = c.Close()
		record(Event{
			Event: "blocked", Host: p.dest.Addr().String(), Port: int(p.dest.Port()), Method: "TCP",
			Reason: "a direct connection, not through the proxy", Refused: true,
		})
		return
	}
	if _, err := c.Write([]byte{deciderTakeOver}); err != nil {
		_ = c.Close()
		return
	}
	_ = c.SetDeadline(time.Time{})
	conn := &firstConn{Conn: c, r: io.MultiReader(bytes.NewReader(p.first), c)}
	select {
	case h.conns <- conn:
	case <-h.done:
		_ = c.Close()
	}
}

// startDecider listens on -decider, for connections to -decider-to, with -decider-token-file's
// token; nil when there's no -decider. Only loopback addresses are taken.
func startDecider() (*handover, netip.AddrPort, []byte, error) {
	var to netip.AddrPort
	tokenFile := deciderTokenFile
	if deciderAddr == "" {
		if deciderTo != "" || tokenFile != "" {
			return nil, to, nil, errors.New("-decider-to and -decider-token-file need -decider")
		}
		return nil, to, nil, nil
	}
	at, err := netip.ParseAddrPort(deciderAddr)
	if err != nil || !at.Addr().IsLoopback() {
		return nil, to, nil, fmt.Errorf("-decider %q: not a loopback address and port", deciderAddr)
	}
	if to, err = netip.ParseAddrPort(deciderTo); err != nil || to.Port() == 0 {
		return nil, to, nil, fmt.Errorf("-decider-to %q: not an address and port", deciderTo)
	}
	to = netip.AddrPortFrom(to.Addr().Unmap(), to.Port())
	if tokenFile == "" {
		return nil, to, nil, errors.New("-decider needs -decider-token-file")
	}
	token, err := readDeciderToken(tokenFile)
	if err != nil {
		return nil, to, nil, err
	}
	ln, err := net.Listen("tcp", at.String()) //nolint:noctx // listens for the proxy's whole life
	if err != nil {
		return nil, to, nil, err
	}
	return newHandover(ln), to, token, nil
}
