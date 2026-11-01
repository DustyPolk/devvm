package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// --- links between VMs ---
//
// The proxy is a second entrance to the VM's links (-links): a request for one of their names and
// ports, CONNECT db:5432 or GET http://db:5432/, goes to that link's socket on the host, which
// devvm _links serves as it serves the link's remote forward, checking each connection there. The
// proxy reads the list devvm _links writes whenever the links it carries change, on every request,
// before any rule: a host and port on it match exactly, after splitHostPort's one normalization
// (lower case, one trailing dot), and the list names each link's short name and its long one
// (NAME.devvm.internal). Its bytes pass as they are: no TLS check or interception, no secret put
// in, no rule asked, and no event of the proxy's own, as devvm _links logs the connection. A
// request names a host and a port, never a socket: the socket is the one the list's folder has for
// this VM's link at that address and port (LinkSocket), so a list holding anything else is
// ignored, and no request reaches another VM's link or any other socket.

var linksFile = "" // -links: the VM's links list, which devvm _links writes; empty: none

// LinkList is the file devvm _links writes for the VM's proxy: the links it carries now.
type LinkList struct {
	Version int         `json:"version"` // LinkListVersion
	Links   []LinkEntry `json:"links"`
}

// LinkListVersion is the list's version: a list of another isn't read.
const LinkListVersion = 1

// LinkEntry is one link the VM's proxy may reach: its names, its address and port in the VM, its
// target (for the log) and its socket on the host (LinkSocket).
type LinkEntry struct {
	Hosts  []string `json:"hosts"`
	Addr   string   `json:"addr"`
	Port   int      `json:"port"`
	Target string   `json:"target"`
	Socket string   `json:"socket"`
}

// linkListMax bounds the list the proxy reads.
const linkListMax = 1 << 20

// LinkSocket is the socket on the host of VM vm's link at addr and port in it, in dir, devvm's run
// folder: named by a hash of the three, as a socket's path is at most 104 characters.
func LinkSocket(dir, vm string, addr netip.Addr, port int) string {
	return filepath.Join(dir, "link-"+LinkHash(vm, "socket", addr.String(), strconv.Itoa(port))+".sock")
}

// LinkHash names one of a link session's files: the first 16 hex digits of the SHA-256 of the VM's
// name and parts, each after a NUL.
func LinkHash(vm string, parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(append([]string{vm}, parts...), "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

// readLinkList reads the list at path: a regular file, not a link, that only its owner may
// write, at most linkListMax long, of LinkListVersion.
func readLinkList(path string) (LinkList, error) {
	// Not blocking: a FIFO there can't hold the request.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) //nolint:gosec // devvm names the list
	if err != nil {
		return LinkList{}, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	switch {
	case err != nil:
		return LinkList{}, err
	case !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0:
		return LinkList{}, fmt.Errorf("%s: not a file only its owner writes", path)
	case fi.Size() > linkListMax:
		return LinkList{}, fmt.Errorf("%s: too long", path)
	}
	b, err := io.ReadAll(io.LimitReader(f, linkListMax+1))
	if err != nil {
		return LinkList{}, err
	}
	var l LinkList
	if err := json.Unmarshal(b, &l); err != nil {
		return LinkList{}, fmt.Errorf("%s: %w", path, err)
	}
	if l.Version != LinkListVersion {
		return LinkList{}, fmt.Errorf("%s: version %d", path, l.Version)
	}
	return l, nil
}

// linkFor returns the VM's link that host and port name, from the list as it is now: its entry,
// with the socket it must have. A host and port on no entry, an entry whose socket isn't this VM's
// link's, and a list that can't be read give none, so the request is the policy's.
func linkFor(host string, port int) (LinkEntry, bool) {
	if linksFile == "" {
		return LinkEntry{}, false
	}
	l, err := readLinkList(linksFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			logf("links: %v", err)
		}
		return LinkEntry{}, false
	}
	for _, e := range l.Links {
		if e.Port != port || !slices.Contains(e.Hosts, host) {
			continue
		}
		a, err := netip.ParseAddr(e.Addr)
		if err != nil || a.String() != e.Addr || !isLinkAddr(a) || port < 1 || port > 65535 ||
			e.Socket != LinkSocket(filepath.Dir(linksFile), vmName, a, port) {
			logf("links: %s:%d: not this VM's link's socket", host, port)
			continue
		}
		return e, true
	}
	return LinkEntry{}, false
}

// isLinkAddr: a is in 127.77.0.0/16, where every link's address in its client is.
func isLinkAddr(a netip.Addr) bool {
	b := a.As16()
	return a.Is4() && b[12] == 127 && b[13] == 77
}

// linkDomain is the domain of the links' long names: a host under it is a link's or nothing.
const linkDomain = ".devvm.internal"

// notALink refuses a host under linkDomain that's none of the VM's links: such a name is never
// the policy's, so no rule or company proxy sends it anywhere.
func notALink(w http.ResponseWriter, method, host string, port int) bool {
	if !strings.HasSuffix(host, linkDomain) && host != strings.TrimPrefix(linkDomain, ".") {
		return false
	}
	record(Event{Event: "blocked", Host: host, Port: port, Method: method, Reason: "not one of this VM's links", Refused: true})
	http.Error(w, "devvm proxy: "+net.JoinHostPort(host, strconv.Itoa(port))+" isn't one of this VM's links", http.StatusForbidden)
	return true
}

// linkDialTimeout bounds how long the proxy waits for a link's socket.
const linkDialTimeout = 10 * time.Second

// dialLink connects to a link's socket. A socket whose backlog is full (a burst of connections
// while devvm _links starts their relays) refuses a connection, or asks for another try, so a few
// more are made, for under a second, as a socket nothing listens on refuses too.
func dialLink(ctx context.Context, sock string) (net.Conn, error) {
	d := net.Dialer{Timeout: linkDialTimeout}
	wait := 10 * time.Millisecond
	for i := 0; ; i++ {
		c, err := d.DialContext(ctx, "unix", sock)
		if err == nil || i == 6 || !errors.Is(err, syscall.ECONNREFUSED) && !errors.Is(err, syscall.EAGAIN) {
			return c, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(wait):
		}
		wait *= 2
	}
}

// linkGone answers a request for a link whose socket doesn't answer (its link removed since the
// list was read, or devvm _links gone) and logs it as devvm _links logs a refusal.
func linkGone(w http.ResponseWriter, host string, port int, e LinkEntry, err error) {
	logf("links: %s:%d: %v", host, port, err)
	record(Event{Event: "link_refused", Host: host, Port: port, Target: e.Target, Reason: linkNotCarried})
	http.Error(w, "devvm proxy: the link "+net.JoinHostPort(host, strconv.Itoa(port))+" isn't carried now", http.StatusBadGateway)
}

// linkNotCarried is the reason of a refusal the proxy logs: the link's socket didn't answer.
const linkNotCarried = "not carried"

// connectLink tunnels a CONNECT for a link to its socket, the bytes as they are.
func connectLink(w http.ResponseWriter, r *http.Request, host string, port int, e LinkEntry) {
	up, err := dialLink(r.Context(), e.Socket)
	if err != nil {
		linkGone(w, host, port, e, err)
		return
	}
	defer func() { _ = up.Close() }()
	client, buf, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = client.Close() }()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go func() {
		_, _ = io.Copy(up, buf.Reader) // also drains anything bufio already read
		if t, ok := up.(interface{ CloseWrite() error }); ok {
			_ = t.CloseWrite()
		}
	}()
	_, _ = io.Copy(client, up)
}

// linkDialError: a link's socket didn't answer.
type linkDialError struct{ err error }

func (e linkDialError) Error() string { return e.err.Error() }

// forwardLink forwards an absolute-form request for a link (plain HTTP) to its socket, a new
// connection for each request, so devvm _links checks each one, and none of the proxy's headers.
func forwardLink(w http.ResponseWriter, r *http.Request, host string, port int, e LinkEntry) {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			c, err := dialLink(ctx, e.Socket)
			if err != nil {
				return nil, linkDialError{err}
			}
			return c, nil
		},
		DisableKeepAlives: true, DisableCompression: true,
	}
	defer tr.CloseIdleConnections()
	rp := &httputil.ReverseProxy{
		Transport:     tr,
		ErrorLog:      logger,
		FlushInterval: -1,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = "http", r.URL.Host
			pr.Out.Header.Del("Proxy-Authorization")
			pr.Out.Header.Del("Proxy-Connection")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if de := (linkDialError{}); errors.As(err, &de) {
				linkGone(w, host, port, e, de.err)
				return
			}
			logf("links: %s:%d: %v", host, port, err)
			http.Error(w, "devvm proxy: the link "+net.JoinHostPort(host, strconv.Itoa(port))+" didn't answer", http.StatusBadGateway)
		},
	}
	// The client's own context: a request it gave up on lets its link connection go, so a target
	// that never answers can't hold the client's links' connections.
	rp.ServeHTTP(w, r)
}
