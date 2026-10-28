package proxy

// A VM whose backend can't reach the host's loopback port from its network (Docker's internal
// networks) reaches the proxy through a relay: devvm _proxy -relay, in a container of its own on the
// VM's network, passes each connection's bytes unread to a unix socket the proxy also listens on
// (-listen-unix), which the backend mounts into the relay's container alone. The relay decides
// nothing: the proxy behind it checks the password and the policy as for any connection.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"sync"
	"time"
)

var (
	listenUnix = "" // -listen-unix: also serve on this unix socket
	relayAddr  = "" // -relay: listen here and pass each connection on to -relay-to
	relayTo    = "" // -relay-to: the proxy's unix socket
)

// relayMax is the most connections a relay carries at once; more wait for one to end.
const relayMax = 512

// relayDial is how long a relay waits for the proxy's socket to answer (Docker Desktop holds a
// connection to a socket path with nothing there until the client gives up).
const relayDial = 10 * time.Second

// listenOn listens on a unix socket at path, only the user's (0600): a socket left there by an
// earlier proxy is removed first, anything else at the path is refused.
func listenOn(path string) (net.Listener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("-listen-unix %s: not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path) //nolint:noctx // listens for the proxy's whole life
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// runRelay listens on addr and passes each connection, both ways and unread, to the unix socket
// at to, until the listener fails.
func runRelay(addr, to string) error {
	if to == "" {
		return errors.New("-relay needs -relay-to")
	}
	ln, err := net.Listen("tcp", addr) //nolint:noctx // listens for the relay's whole life
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	logf("relaying %s to %s", ln.Addr(), to)
	slots := make(chan struct{}, relayMax)
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		slots <- struct{}{}
		go func() {
			defer func() { <-slots }()
			relayConn(c, to)
		}()
	}
}

// relayConn passes c's bytes to a new connection to the socket at to and back, until both
// directions end.
func relayConn(c net.Conn, to string) {
	defer func() { _ = c.Close() }()
	up, err := net.DialTimeout("unix", to, relayDial) //nolint:noctx // a connection's own dial
	if err != nil {
		logf("relay: %v", err)
		return
	}
	defer func() { _ = up.Close() }()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pipe(up, c) }()
	go func() { defer wg.Done(); pipe(c, up) }()
	wg.Wait()
}

// pipe copies src to dst, then closes dst's writing side (or dst, when it has none), so the other
// end sees the end of what src sent.
func pipe(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
		return
	}
	_ = dst.Close()
}
