package proxy

// The upstream: a company proxy the host has to go through (Config.Upstream). The proxy still decides
// every request by the name the VM asked for, then reaches allowed hosts through the upstream: a
// CONNECT tunnel through HTTPS_PROXY's for tunnels and the connections it makes to put secrets in,
// an absolute-form request to HTTP_PROXY's for plain HTTP. The upstream gets the host's name, not an
// address, and resolves it itself, so the floor can only judge what the host's own resolver answers
// (floorUpstream). Hosts NO_PROXY names, localhost and loopback addresses are reached directly. The
// upstream's credentials go only to the upstream, in Proxy-Authorization, and are never logged: the
// log names it by host:port.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// upstream is one company proxy: its address and, from the URL's user and password, the
// Proxy-Authorization it's sent ("" for none).
type upstream struct {
	addr string // host:port
	auth string
}

var (
	upUnresolved = false   // Config.UpstreamUnresolved: names the host can't resolve go upstream even when only ** allows them
	upHTTPS      *upstream // tunnels and the connections that put secrets in; nil: direct
	upHTTP       *upstream // plain HTTP requests; nil: direct
	noProxy      []noProxyRule
)

// ErrUpstreamScheme is ParseUpstream's error for a proxy that isn't http://.
var ErrUpstreamScheme = errors.New("only http:// proxies can be an upstream")

// ParseUpstream reads a proxy URL as Go's own clients do: http://user:pass@host:port, or
// host:port alone (http:// assumed); port 80 when it names none. Only http:// proxies are
// supported: https:// and socks5:// ones get ErrUpstreamScheme, with the scheme in the error.
// Errors never hold the URL, which may hold a password.
func ParseUpstream(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		if u2, err2 := url.Parse("http://" + raw); err2 == nil && u2.Host != "" {
			u, err = u2, nil
		}
	}
	if err != nil || u.Host == "" {
		return nil, errors.New("not a proxy URL")
	}
	if s := strings.ToLower(u.Scheme); s != "http" {
		return nil, fmt.Errorf("%w: %s://", ErrUpstreamScheme, s)
	}
	if u.Hostname() == "" || strings.ContainsAny(u.Host, " \t\r\n") {
		return nil, errors.New("not a proxy URL")
	}
	if u.Port() != "" {
		if n, err := strconv.Atoi(u.Port()); err != nil || n < 1 || n > 65535 {
			return nil, errors.New("not a proxy URL")
		}
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" {
		return nil, errors.New("not a proxy URL")
	}
	return u, nil
}

// newUpstream makes an upstream of a proxy URL ("": none).
func newUpstream(raw string) (*upstream, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	u, err := ParseUpstream(raw)
	if err != nil {
		return nil, err
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	up := &upstream{addr: net.JoinHostPort(u.Hostname(), port)}
	if u.User != nil {
		pass, _ := u.User.Password()
		up.auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(u.User.Username()+":"+pass))
	}
	return up, nil
}

// loadUpstream takes the config's company proxy (Config.Upstream): the host's HTTPS_PROXY,
// HTTP_PROXY and NO_PROXY.
func loadUpstream(u Upstream) error {
	var err error
	if upHTTPS, err = newUpstream(u.HTTPS); err != nil {
		return fmt.Errorf("upstream: HTTPS_PROXY: %w", err)
	}
	if upHTTP, err = newUpstream(u.HTTP); err != nil {
		return fmt.Errorf("upstream: HTTP_PROXY: %w", err)
	}
	noProxy = parseNoProxy(u.NoProxy)
	return nil
}

// upstreamNames: the upstreams, by host:port only, for the proxy's log.
func upstreamNames() string {
	name := func(u *upstream) string {
		if u == nil {
			return "none"
		}
		return u.addr
	}
	return "https through " + name(upHTTPS) + ", http through " + name(upHTTP)
}

// --- NO_PROXY ---

// noProxyRule is one NO_PROXY entry, read as Go's clients read it: "*" (every host), an address
// or CIDR, or a name, which covers its subdomains too ("example.com"), or only them
// (".example.com", or "*.example.com"); any of them with a port, which it's then limited to.
type noProxyRule struct {
	all    bool
	prefix netip.Prefix
	name   string // lowercased; a leading "." matches subdomains only
	port   int    // 0: any
}

func parseNoProxy(s string) []noProxyRule {
	var out []noProxyRule
	for e := range strings.SplitSeq(s, ",") {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if e == "*" {
			out = append(out, noProxyRule{all: true})
			continue
		}
		r := noProxyRule{}
		if p, err := netip.ParsePrefix(e); err == nil {
			r.prefix = p.Masked()
			out = append(out, r)
			continue
		}
		if h, p, err := net.SplitHostPort(e); err == nil {
			if n, err := strconv.Atoi(p); err == nil {
				e, r.port = h, n
			}
		}
		e = strings.Trim(e, "[]")
		if a, err := netip.ParseAddr(e); err == nil {
			r.prefix = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
			out = append(out, r)
			continue
		}
		if strings.HasPrefix(e, "*.") {
			e = e[1:]
		}
		r.name = strings.TrimSuffix(e, ".")
		if r.name != "" && r.name != "." {
			out = append(out, r)
		}
	}
	return out
}

func (r noProxyRule) matches(host string, port int) bool {
	if r.all {
		return true
	}
	if r.port != 0 && r.port != port {
		return false
	}
	if r.prefix.IsValid() {
		a, err := netip.ParseAddr(strings.Trim(host, "[]"))
		return err == nil && r.prefix.Contains(a.Unmap())
	}
	if strings.HasPrefix(r.name, ".") {
		return strings.HasSuffix(host, r.name)
	}
	return host == r.name || strings.HasSuffix(host, "."+r.name)
}

// local: names and addresses that are always the host's own: never sent upstream, so the floor
// judges them as ever.
func local(host string) bool {
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return a.Unmap().IsLoopback() || a.IsUnspecified()
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost")
}

// upstreamFor: the upstream host:port goes through, nil for direct. plain is a plain HTTP request
// (HTTP_PROXY's); anything else is a tunnel (HTTPS_PROXY's). host is canonical (splitHostPort).
func upstreamFor(host string, port int, plain bool) *upstream {
	up := upHTTPS
	if plain {
		up = upHTTP
	}
	if up == nil || local(host) {
		return nil
	}
	for _, r := range noProxy {
		if r.matches(host, port) {
			return nil
		}
	}
	return up
}

// --- through it ---

// errUnresolved is floorUpstream's refusal of a name the host can't resolve that only ** allows.
var errUnresolved = errors.New("this machine can't resolve it, and only ** allows it; allow the name itself to reach it through the company proxy")

// floorUpstream is the floor for a host sent upstream, which resolves it: an address is judged as
// ever; a name by what the host's resolver answers, if it answers, refusing it when any answer is
// internal or denied (the upstream may connect to any of them). A name the host can't resolve
// (common behind a company proxy) goes upstream unjudged only when the policy names it, as itself
// or under *.domain: one that only ** allows is refused (errUnresolved), so ** alone never sends
// the company proxy a name it may resolve to its own network's hosts. With Config.UpstreamUnresolved
// (a build whose VM reaches internal addresses anyway), any such name goes upstream.
func floorUpstream(ctx context.Context, host string, port int) error {
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		_, err := resolve(ctx, host, port)
		return err
	}
	lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(lctx, "ip", host)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !upUnresolved && !pol.byName(host, port) {
			return errUnresolved
		}
		return nil
	}
	for _, a := range ips {
		a = a.Unmap()
		if pol.addrDenied(a, port) || internal(a) && !pol.addrAllowed(a, port) {
			return errFloor
		}
	}
	return nil
}

// dialUpstream connects to the upstream itself.
func (u *upstream) dialUpstream(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: 15 * time.Second}
	c, err := d.DialContext(ctx, "tcp", u.addr)
	if err != nil {
		return nil, fmt.Errorf("the upstream proxy %s: %w", u.addr, err)
	}
	return c, nil
}

// connect opens a tunnel to host:port through the upstream: CONNECT by name, with its credentials.
func (u *upstream) connect(ctx context.Context, host string, port int) (net.Conn, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port %d", port)
	}
	c, err := u.dialUpstream(ctx)
	if err != nil {
		return nil, err
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	req := "CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n"
	if u.auth != "" {
		req += "Proxy-Authorization: " + u.auth + "\r\n"
	}
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	stop := context.AfterFunc(ctx, func() { _ = c.SetDeadline(time.Now()) })
	defer stop()
	if _, err := c.Write([]byte(req + "\r\n")); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("the upstream proxy %s: %w", u.addr, err)
	}
	// The upstream's answer is read only as far as maxConnectReply: one that never ends its
	// headers fails rather than fill memory.
	br := bufio.NewReader(io.LimitReader(c, maxConnectReply))
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("the upstream proxy %s: %w", u.addr, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = c.Close()
		return nil, fmt.Errorf("the upstream proxy %s answered %d", u.addr, resp.StatusCode)
	}
	if !stop() {
		_ = c.Close()
		return nil, ctx.Err()
	}
	_ = c.SetDeadline(time.Time{})
	if n := br.Buffered(); n > 0 {
		// What came after the answer is the tunnel's: read before the rest, which the limit
		// doesn't cover.
		rest, _ := br.Peek(n)
		return bufConn{c, bufio.NewReader(io.MultiReader(bytes.NewReader(bytes.Clone(rest)), c))}, nil
	}
	return c, nil
}

// maxConnectReply is as much of an upstream's answer to CONNECT as is read: its status line and
// headers.
const maxConnectReply = 64 << 10

// dialOut connects to host:port for a tunnel or a TLS connection: through the upstream when one
// applies, else directly to an address the floor allows.
func dialOut(ctx context.Context, host string, port int) (net.Conn, error) {
	if up := upstreamFor(host, port, false); up != nil {
		return up.connect(ctx, host, port)
	}
	return dial(ctx, host, port)
}
