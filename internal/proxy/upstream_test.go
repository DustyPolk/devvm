package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const upPassword = "s3cret-upstream-pw"

// fakeUpstream is a company proxy for tests: it records each request's line and
// Proxy-Authorization, and answers status; a CONNECT it accepts goes to target, whatever it names.
type fakeUpstream struct {
	ln     net.Listener
	target string
	status int
	mu     sync.Mutex
	seen   []string // "METHOD request-URI auth"
}

func newFakeUpstream(t *testing.T, target string, status int) *fakeUpstream {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	f := &fakeUpstream{ln: ln, target: target, status: status}
	t.Cleanup(func() { _ = ln.Close() })
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

func (f *fakeUpstream) serve(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.seen = append(f.seen, req.Method+" "+req.RequestURI+" "+req.Header.Get("Proxy-Authorization"))
	f.mu.Unlock()
	if f.status != http.StatusOK {
		fmt.Fprintf(c, "HTTP/1.1 %d No\r\nContent-Length: 0\r\n\r\n", f.status)
		return
	}
	if req.Method != http.MethodConnect {
		fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: 8\r\nConnection: close\r\n\r\nupstream")
		return
	}
	t, err := net.Dial("tcp", f.target) //nolint:noctx // a test's fake proxy
	if err != nil {
		return
	}
	defer t.Close()
	fmt.Fprint(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	go func() { _, _ = io.Copy(t, br) }()
	_, _ = io.Copy(c, t)
}

func (f *fakeUpstream) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

// useUpstream points both of s's upstreams at f, with the test's credentials, and NO_PROXY at
// noProxyList.
func useUpstream(t *testing.T, s *server, f *fakeUpstream, noProxyList string) {
	t.Helper()
	u := "http://corp-user:" + upPassword + "@" + f.ln.Addr().String()
	ups, err := newUpstreams(Upstream{HTTPS: u, HTTP: u, NoProxy: noProxyList}, nil)
	must(t, err)
	s.egress.up = ups
}

func authOf(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestParseUpstream(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		raw, host string
		scheme    bool // ErrUpstreamScheme
	}{
		{"http://user:pw@proxy.corp:8080", "proxy.corp:8080", false},
		{"proxy.corp:3128", "proxy.corp:3128", false},
		{"proxy.corp", "proxy.corp", false},
		{"HTTP://10.0.0.1:8080/", "10.0.0.1:8080", false},
		{"https://user:pw@proxy.corp:443", "", true},
		{"socks5://user:pw@proxy.corp:1080", "", true},
		{"socks5h://proxy.corp:1080", "", true},
		{"http://proxy.corp:99999", "", false},
		{"http://proxy.corp:8080/path", "", false},
		{"", "", false},
	} {
		u, err := ParseUpstream(c.raw)
		switch {
		case c.scheme:
			if !errors.Is(err, ErrUpstreamScheme) {
				t.Errorf("%q: %v, want the scheme refused", c.raw, err)
			}
		case c.host == "":
			if err == nil || errors.Is(err, ErrUpstreamScheme) {
				t.Errorf("%q: %v, want it refused", c.raw, err)
			}
		case err != nil || u.Host != c.host:
			t.Errorf("%q: %v, %v; want %s", c.raw, u, err, c.host)
		}
		if err != nil && strings.Contains(err.Error(), "pw") {
			t.Errorf("%q: the error holds the password: %v", c.raw, err)
		}
	}
	up, err := newUpstream("proxy.corp", "")
	if err != nil || up.addr != "proxy.corp:80" || up.auth != "" {
		t.Errorf("no port: %+v %v", up, err)
	}
	up, err = newUpstream("http://u%40corp:p%3Aw@proxy.corp:8080", "")
	if err != nil || up.auth != authOf("u@corp", "p:w") {
		t.Errorf("escaped credentials: %+v %v", up, err)
	}
	// A URL's own password wins over the secret's; one without a user gets none.
	up, err = newUpstream("http://u:own@proxy.corp:8080", "kept")
	if err != nil || up.auth != authOf("u", "own") {
		t.Errorf("own password: %+v %v", up, err)
	}
	up, err = newUpstream("http://proxy.corp:8080", "kept")
	if err != nil || up.auth != "" {
		t.Errorf("no user: %+v %v", up, err)
	}
}

// TestUpstreamSecret: config.toml's proxy names a user and no password, and the proxy looks the
// password up as it starts, through IO.Secret, as a secret devvm keeps. A lookup that fails, or
// none, fails the start.
func TestUpstreamSecret(t *testing.T) {
	t.Parallel()
	var asked []Binding
	lookup := func(_ context.Context, b Binding) (string, error) {
		asked = append(asked, b)
		if b.Name != UpstreamSecret {
			return "", errors.New("no such secret")
		}
		return " s3cret\n", nil
	}
	ups, err := newUpstreams(Upstream{HTTPS: "http://alice@proxy.corp:3128", HTTP: "proxy.corp:3128", Secret: UpstreamSecret}, lookup)
	must(t, err)
	if ups.https.auth != authOf("alice", "s3cret") || ups.http.auth != "" {
		t.Errorf("auth: %q, %q", ups.https.auth, ups.http.auth)
	}
	if len(asked) != 1 || asked[0].Name != UpstreamSecret || asked[0].Kind != KindSecret {
		t.Errorf("asked %+v", asked)
	}
	for _, c := range []struct {
		name   string
		secret string
		lookup Lookup
	}{
		{"no value", "other", lookup},
		{"no lookup", UpstreamSecret, nil},
		{"empty", UpstreamSecret, func(context.Context, Binding) (string, error) { return " ", nil }},
	} {
		if _, err := newUpstreams(Upstream{HTTPS: "http://alice@proxy.corp:3128", Secret: c.secret}, c.lookup); err == nil {
			t.Errorf("%s: started", c.name)
		}
	}
	// Without a Secret, nothing is looked up.
	asked = nil
	if _, err := newUpstreams(Upstream{HTTPS: "http://alice@proxy.corp:3128"}, lookup); err != nil || asked != nil {
		t.Errorf("no secret: %v, asked %+v", err, asked)
	}
}

func TestNoProxy(t *testing.T) {
	t.Parallel()
	ups := upstreams{
		https: &upstream{addr: "https.corp:8080"}, http: &upstream{addr: "http.corp:8080"},
		noProxy: parseNoProxy(" Corp.Example , .internal.test,*.wild.test, 10.0.0.0/8, 192.0.2.7, git.test:22, [2001:db8::1]"),
	}
	for _, c := range []struct {
		host   string
		port   int
		plain  bool
		direct bool
	}{
		{"github.com", 443, false, false},
		{"github.com", 80, true, false},
		{"corp.example", 443, false, true},     // a name covers itself
		{"api.corp.example", 443, false, true}, // and its subdomains
		{"xcorp.example", 443, false, false},   // not another name ending the same
		{"internal.test", 443, false, false},   // a leading dot: subdomains only
		{"a.internal.test", 443, false, true},
		{"a.wild.test", 443, false, true}, // *. reads as .
		{"wild.test", 443, false, false},
		{"10.1.2.3", 443, false, true}, // CIDR
		{"192.0.2.7", 443, false, true},
		{"192.0.2.8", 443, false, false},
		{"git.test", 22, false, true}, // a port limits it
		{"git.test", 443, false, false},
		{"2001:db8::1", 443, false, true},
		{"localhost", 8080, false, true}, // always the host's own
		{"a.localhost", 8080, false, true},
		{"127.0.0.1", 8080, true, true},
		{"::1", 8080, false, true},
	} {
		up := ups.route(c.host, c.port, c.plain)
		if (up == nil) != c.direct {
			t.Errorf("%s:%d (plain %v): upstream %v, want direct %v", c.host, c.port, c.plain, up, c.direct)
		}
		if up != nil && c.plain != (up == ups.http) {
			t.Errorf("%s:%d (plain %v): the wrong upstream, %s", c.host, c.port, c.plain, up.addr)
		}
	}
	ups.noProxy = parseNoProxy("*")
	if ups.route("github.com", 443, false) != nil {
		t.Error("NO_PROXY=* sent a host upstream")
	}
	// Without HTTP_PROXY, plain HTTP goes direct; tunnels still go upstream.
	ups.noProxy, ups.http = nil, nil
	if ups.route("github.com", 80, true) != nil || ups.route("github.com", 80, false) == nil {
		t.Error("HTTPS_PROXY alone: plain HTTP should go direct, tunnels upstream")
	}
}

// A tunnel to an allowed host goes through the upstream: CONNECT by the name the VM asked for, with
// the upstream's credentials, never the VM's; a denied host, or a private address, never reaches it.
func TestUpstreamTunnel(t *testing.T) {
	t.Parallel()
	echo, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = echo.Close() })
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(c, c) }()
		}
	}()
	f := newFakeUpstream(t, echo.Addr().String(), http.StatusOK)
	s := testServer(t, Config{Password: "k3y"}, "allowed.test\n**:5432\n!denied.test\n10.1.2.3\n")
	useUpstream(t, s, f, "")
	out := logs(s)
	addr := listen(t, s)
	vmAuth := "Proxy-Authorization: " + authOf("devvm", "k3y") + "\r\n"

	c, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
	must(t, err)
	defer c.Close()
	fmt.Fprintf(c, "CONNECT allowed.test:443 HTTP/1.1\r\nHost: allowed.test:443\r\n%s\r\n", vmAuth)
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	must(t, err)
	_ = resp.Body.Close() // a CONNECT's 200 has none: the tunnel follows
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT: %d", resp.StatusCode)
	}
	fmt.Fprint(c, "hello")
	got := make([]byte, 5)
	if _, err := io.ReadFull(br, got); err != nil || string(got) != "hello" {
		t.Errorf("through the tunnel: %q %v", got, err)
	}
	if reqs := f.requests(); len(reqs) != 1 || reqs[0] != "CONNECT allowed.test:443 "+authOf("corp-user", upPassword) {
		t.Errorf("the upstream saw %q", reqs)
	}

	// Refused before the upstream: a denied host, a host the policy doesn't allow, and a private
	// address the policy allows but the floor still judges.
	for _, target := range []string{"denied.test:443", "other.test:443", "10.1.2.4:5432", "127.0.0.1:5432"} {
		code, _ := roundTrip(t, addr, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n"+vmAuth+"\r\n")
		if code != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", target, code)
		}
	}
	if n := len(f.requests()); n != 1 {
		t.Errorf("refused hosts reached the upstream: %q", f.requests())
	}
	// An allowed address goes upstream too, by its address.
	c2, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
	must(t, err)
	fmt.Fprintf(c2, "CONNECT 10.1.2.3:443 HTTP/1.1\r\nHost: 10.1.2.3:443\r\n%s\r\n", vmAuth)
	resp, err = http.ReadResponse(bufio.NewReader(c2), &http.Request{Method: http.MethodConnect})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Errorf("the allowed address: %v %v", resp, err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}
	_ = c2.Close()
	if reqs := f.requests(); len(reqs) != 2 || !strings.HasPrefix(reqs[1], "CONNECT 10.1.2.3:443 ") {
		t.Errorf("the allowed address: %q", reqs)
	}
	l := out.String()
	if strings.Contains(l, upPassword) || strings.Contains(l, authOf("corp-user", upPassword)) {
		t.Errorf("the logs hold the upstream's credentials: %s", l)
	}
	if !strings.Contains(l, `"event":"tunnel","host":"allowed.test","port":443`) || !strings.Contains(l, `"upstream":"`+f.ln.Addr().String()+`"`) {
		t.Errorf("the event log doesn't name the upstream: %s", l)
	}
}

// A name this machine can't resolve goes upstream only when the policy names it, as itself or
// under *.domain; one that only ** allows is refused before the upstream sees it. (.test names
// never resolve.)
func TestUpstreamUnresolved(t *testing.T) {
	t.Parallel()
	echo, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	must(t, err)
	t.Cleanup(func() { _ = echo.Close() })
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	f := newFakeUpstream(t, echo.Addr().String(), http.StatusOK)
	s := testServer(t, Config{}, "**\nnamed.test\n*.wild.test\n!denied.wild.test\n")
	useUpstream(t, s, f, "")
	out := logs(s)
	addr := listen(t, s)
	for _, tc := range []struct {
		target string
		code   int
	}{
		{"named.test:443", http.StatusOK},
		{"a.wild.test:443", http.StatusOK},
		{"only-star.test:443", http.StatusForbidden},
		{"wild.test:443", http.StatusForbidden}, // *.wild.test names its subdomains alone
		{"denied.wild.test:443", http.StatusForbidden},
		{"named.test:8443", http.StatusForbidden}, // named for 443 and 80 only
	} {
		// Only the answer: an open tunnel waits for a ClientHello.
		c, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
		must(t, err)
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", tc.target, tc.target)
		resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: http.MethodConnect})
		if err != nil || resp.StatusCode != tc.code {
			t.Errorf("%s: %v %v, want %d", tc.target, resp, err, tc.code)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		_ = c.Close()
	}
	seen := strings.Join(f.requests(), "\n")
	if !strings.Contains(seen, "CONNECT named.test:443 ") || !strings.Contains(seen, "CONNECT a.wild.test:443 ") ||
		strings.Contains(seen, "only-star") || strings.Contains(seen, "CONNECT wild.test") || strings.Contains(seen, "denied") {
		t.Errorf("the upstream saw %q", f.requests())
	}
	if !strings.Contains(out.String(), "only ** allows it") {
		t.Errorf("the refusal isn't logged: %s", out.String())
	}
	// Plain HTTP the same.
	code, _ := roundTrip(t, addr, "GET http://only-star.test/ HTTP/1.1\r\nHost: only-star.test\r\n\r\n")
	if code != http.StatusForbidden || strings.Contains(strings.Join(f.requests(), "\n"), "only-star") {
		t.Errorf("plain HTTP: %d, the upstream saw %q", code, f.requests())
	}
	// upstream_unresolved (a build's proxy): ** sends it upstream; a denied name stays refused.
	s.egress.up.unresolved = true
	for _, tc := range []struct {
		target string
		code   int
	}{
		{"only-star.test:443", http.StatusOK},
		{"denied.wild.test:443", http.StatusForbidden},
	} {
		c, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
		must(t, err)
		fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", tc.target, tc.target)
		resp, err := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: http.MethodConnect})
		if err != nil || resp.StatusCode != tc.code {
			t.Errorf("upstream_unresolved %s: %v %v, want %d", tc.target, resp, err, tc.code)
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		_ = c.Close()
	}
}

// The upstream's answer to CONNECT is read only so far: headers that never end fail the tunnel.
// What follows an answer that ends is the tunnel's, past the limit too.
func TestUpstreamConnectReplyLimit(t *testing.T) {
	t.Parallel()
	serve := func(answer func(net.Conn)) *upstream {
		ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
		must(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		go func() {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
			if _, err := http.ReadRequest(bufio.NewReader(c)); err != nil {
				return
			}
			answer(c)
		}()
		return &upstream{addr: ln.Addr().String()}
	}
	endless := serve(func(c net.Conn) {
		_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\n")
		line := "X-Pad: " + strings.Repeat("a", 1000) + "\r\n"
		for range 200 {
			if _, err := io.WriteString(c, line); err != nil {
				return
			}
		}
	})
	if c, err := endless.connect(t.Context(), "a.test", 443); err == nil {
		_ = c.Close()
		t.Error("an answer past the limit was taken")
	}
	big := strings.Repeat("t", 2*maxConnectReply)
	tunnel := serve(func(c net.Conn) {
		_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n"+big)
	})
	c, err := tunnel.connect(t.Context(), "a.test", 443)
	must(t, err)
	defer c.Close()
	got, err := io.ReadAll(c)
	if err != nil || string(got) != big {
		t.Errorf("the tunnel's data: %d bytes, %v; want %d", len(got), err, len(big))
	}
}

// NO_PROXY's hosts are reached directly: the upstream never sees them.
func TestUpstreamNoProxyDirect(t *testing.T) {
	t.Parallel()
	f := newFakeUpstream(t, "127.0.0.1:9", http.StatusOK)
	s := testServer(t, Config{}, "allowed.test\n")
	useUpstream(t, s, f, "*")
	// Direct, it's resolved here: .test names don't resolve, so it's refused, unseen upstream.
	code, body := roundTrip(t, listen(t, s), "CONNECT allowed.test:443 HTTP/1.1\r\nHost: allowed.test:443\r\n\r\n")
	if code != http.StatusForbidden || !strings.Contains(body, "can't resolve") || len(f.requests()) != 0 {
		t.Errorf("NO_PROXY=*: %d %q, upstream saw %q", code, body, f.requests())
	}
}

// Plain HTTP goes to HTTP_PROXY's upstream in absolute form, with its credentials in place of the
// VM's; an upstream that refuses is reported to the VM without them.
func TestUpstreamPlainHTTP(t *testing.T) {
	t.Parallel()
	f := newFakeUpstream(t, "", http.StatusOK)
	s := testServer(t, Config{Password: "k3y"}, "allowed.test\n")
	useUpstream(t, s, f, "")
	out := logs(s)
	addr := listen(t, s)
	pu, _ := url.Parse("http://devvm:k3y@" + addr)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://allowed.test/x?y=1", nil)
	resp, err := client.Do(req)
	must(t, err)
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(b) != "upstream" {
		t.Errorf("GET: %d %q", resp.StatusCode, b)
	}
	if reqs := f.requests(); len(reqs) != 1 || reqs[0] != "GET http://allowed.test/x?y=1 "+authOf("corp-user", upPassword) {
		t.Errorf("the upstream saw %q", reqs)
	}
	if l := out.String(); strings.Contains(l, upPassword) || !strings.Contains(l, `"upstream":"`+f.ln.Addr().String()+`"`) {
		t.Errorf("the log: %s", l)
	}
	// The upstream gets the host that was checked, canonical, whatever the VM's spelling.
	if c, _ := roundTrip(t, addr,
		"GET http://ALLOWED.test.:8080/y HTTP/1.1\r\nHost: x\r\nProxy-Authorization: "+authOf("devvm", "k3y")+"\r\nConnection: close\r\n\r\n"); c != http.StatusForbidden {
		t.Errorf("a port the policy doesn't allow: %d", c)
	}
	s.egress.pol = writePolicy(t, "allowed.test\nallowed.test:8080\n")
	code, _ := roundTrip(t, addr,
		"GET http://ALLOWED.test.:8080/y HTTP/1.1\r\nHost: x\r\nProxy-Authorization: "+authOf("devvm", "k3y")+"\r\nConnection: close\r\n\r\n")
	if reqs := f.requests(); code != http.StatusOK || len(reqs) != 2 || !strings.HasPrefix(reqs[1], "GET http://allowed.test:8080/y ") {
		t.Errorf("an odd spelling: %d, the upstream saw %q", code, reqs)
	}

	// An upstream that refuses: the tunnel is refused, naming it by host:port alone.
	f.status = http.StatusProxyAuthRequired
	code, body := roundTrip(t, addr,
		"CONNECT allowed.test:443 HTTP/1.1\r\nHost: allowed.test:443\r\nProxy-Authorization: "+authOf("devvm", "k3y")+"\r\n\r\n")
	if code != http.StatusForbidden || !strings.Contains(body, "answered 407") || strings.Contains(body, upPassword) {
		t.Errorf("refused upstream: %d %q", code, body)
	}
}

// A host with a bound secret is intercepted, and its request goes on through the upstream's tunnel
// over TLS checked against the user's CAs, with the secret put in.
func TestUpstreamIntercept(t *testing.T) {
	t.Parallel()
	s, _, file, c := interceptSetup(t, func(http.ResponseWriter, *http.Request) {})
	// The fake upstream sends every tunnel to a TLS server whose certificate, from the same CA as
	// the user's, names "intercepted.test", which the VM asks for, bound and allowed.
	srvCert, _ := s.ca.leafFor("intercepted.test")
	tlsSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", r.Host, r.Header.Get("Authorization")) //nolint:gosec // a test echo server
	}))
	tlsSrv.TLS = &tls.Config{Certificates: []tls.Certificate{*srvCert}}
	tlsSrv.StartTLS()
	t.Cleanup(tlsSrv.Close)
	f := newFakeUpstream(t, tlsSrv.Listener.Addr().String(), http.StatusOK)
	useUpstream(t, s, f, "")
	s.egress.pol = writePolicy(t, "intercepted.test\n")
	writeBindings(t, file, bound("intercepted.test"))
	later := time.Now().Add(time.Second)
	must(t, os.Chtimes(file, later, later))
	code, body := get(t, c, "https://intercepted.test/x", "")
	if code != http.StatusOK || body != "intercepted.test|Bearer real" {
		t.Errorf("through the upstream: %d %q", code, body)
	}
	if reqs := f.requests(); len(reqs) != 1 || reqs[0] != "CONNECT intercepted.test:443 "+authOf("corp-user", upPassword) {
		t.Errorf("the upstream saw %q", reqs)
	}
	// Without the user's CA, the upstream's certificate isn't trusted: nothing is sent.
	s.upstreamCAs = x509.NewCertPool()
	f.mu.Lock()
	f.seen = nil
	f.mu.Unlock()
	c2 := &http.Client{Transport: c.Transport.(*http.Transport).Clone()}
	if code, _ := get(t, c2, "https://intercepted.test/x", ""); code != http.StatusBadGateway {
		t.Errorf("an untrusted certificate: %d, want 502", code)
	}
}
