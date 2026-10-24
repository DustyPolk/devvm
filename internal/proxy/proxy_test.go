package proxy

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func writePolicy(t *testing.T, rules string) *policy {
	t.Helper()
	f := filepath.Join(t.TempDir(), "policy")
	if err := os.WriteFile(f, []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	return newPolicy([]string{f, filepath.Join(t.TempDir(), "missing")})
}

func TestNewBlocks(t *testing.T) {
	rs := []numbered{
		{1, Event{Host: "old.test", Port: 443, Reason: "not in the allowlist"}},
		{2, Event{Host: "registry.example.com", Port: 443, Reason: "not in the allowlist"}},
		{3, Event{Host: "registry.example.com", Port: 443, Reason: "not in the allowlist"}},
		{4, Event{Host: "db.test", Port: 5432, Reason: "not in the allowlist"}},
	}
	newer := "4\n" +
		"devvm: blocked registry.example.com:443 (not in the allowlist)\n" +
		"devvm: blocked db.test:5432 (not in the allowlist)\n" +
		"To allow, run this on the host:\n" +
		"devvm policy allow registry.example.com db.test:5432\n"
	all := "4\n" +
		"devvm: blocked old.test:443 (not in the allowlist)\n" +
		"devvm: blocked registry.example.com:443 (not in the allowlist)\n" +
		"devvm: blocked db.test:5432 (not in the allowlist)\n" +
		"To allow, run this on the host:\n" +
		"devvm policy allow old.test registry.example.com db.test:5432\n"
	for _, c := range []struct{ after, want string }{
		{"1", newer}, // after the prompt that saw block 1: each new rule once
		{"4", "4\n"}, // nothing new
		{"", "4\n"},  // a new shell: only the count
		{"x", "4\n"}, // not a number: the same
		{"9", all},   // past the count: the proxy restarted, so everything it has
	} {
		got := newBlocks(rs, 4, c.after)
		if got != c.want {
			t.Errorf("after=%q:\n%s\nwant:\n%s", c.after, got, c.want)
		}
	}
	// A refused request has no rule to allow it.
	rs = append(rs, numbered{5, Event{Host: "api.github.com", Port: 443, Method: "POST", Path: "/user/keys", Reason: "makes a key", Refused: true}})
	if got, want := newBlocks(rs, 5, "4"), "5\ndevvm: refused POST api.github.com/user/keys (makes a key)\n"; got != want {
		t.Errorf("refused:\n%s\nwant:\n%s", got, want)
	}
}

func TestDecide(t *testing.T) {
	p := writePolicy(t, `
# comment
example.com
*.github.com          # any depth, not the apex
github.com:22
!evil.github.com
10.1.2.3
192.168.0.0/16:8080
**:9999
`)
	cases := []struct {
		host string
		port int
		want bool
	}{
		{"example.com", 443, true},
		{"EXAMPLE.com", 80, true},
		{"example.com.", 443, true},
		{"example.com", 8443, false},    // no port in the rule: 443 and 80 only
		{"www.example.com", 443, false}, // exact only
		{"api.github.com", 443, true},   // *. covers subdomains
		{"a.b.github.com", 443, true},   // at any depth
		{"github.com", 443, false},      // but not the apex
		{"github.com", 22, true},        // port rule
		{"evil.github.com", 443, false}, // deny wins
		{"10.1.2.3", 443, true},         // address rule
		{"10.1.2.4", 443, false},
		{"192.168.5.5", 8080, true}, // cidr with port
		{"192.168.5.5", 443, false},
		{"anything.test", 9999, true}, // ** with a port
		{"anything.test", 443, false},
	}
	for _, c := range cases {
		if got, why := p.decide(c.host, c.port); got != c.want {
			t.Errorf("decide(%s, %d) = %v (%s), want %v", c.host, c.port, got, why, c.want)
		}
	}
}

func TestPolicyReload(t *testing.T) {
	f := filepath.Join(t.TempDir(), "policy")
	must(t, os.WriteFile(f, []byte("a.test\n"), 0o600))
	p := newPolicy([]string{f})
	if ok, _ := p.decide("b.test", 443); ok {
		t.Fatal("b.test allowed before it was added")
	}
	must(t, os.WriteFile(f, []byte("a.test\nb.test\n"), 0o600))
	must(t, os.Chtimes(f, time.Now().Add(time.Second), time.Now().Add(time.Second)))
	if ok, why := p.decide("b.test", 443); !ok {
		t.Fatalf("b.test still blocked after the file changed: %s", why)
	}
	// A file that changed but can't be read (mid-rewrite, say) keeps its last rules: its denies too.
	must(t, os.WriteFile(f, []byte("**\n!c.test\n"), 0o600))
	must(t, os.Chtimes(f, time.Now().Add(2*time.Second), time.Now().Add(2*time.Second)))
	p.decide("x.test", 443)
	must(t, os.Chmod(f, 0))
	must(t, os.Chtimes(f, time.Now().Add(3*time.Second), time.Now().Add(3*time.Second)))
	t.Cleanup(func() { _ = os.Chmod(f, 0o600) })
	if ok, _ := p.decide("c.test", 443); ok {
		t.Error("c.test's deny was dropped while its file couldn't be read")
	}
}

func TestInternal(t *testing.T) {
	for _, s := range []string{
		"127.0.0.1", "::1", "10.0.0.1", "172.16.3.4", "192.168.1.1", "169.254.169.254",
		"100.64.0.1", "fd00::1", "fe80::1", "0.0.0.0", "::ffff:127.0.0.1", "224.0.0.1",
		"0.1.2.3", "192.0.0.8", "198.18.0.1", "240.0.0.1", "64:ff9b::10.0.0.1", "64:ff9b::127.0.0.1", "64:ff9b:1::1",
		"::7f00:1", "::a00:1", "fec0::1",
	} {
		if !internal(netip.MustParseAddr(s)) {
			t.Errorf("%s should be internal", s)
		}
	}
	for _, s := range []string{"1.1.1.1", "140.82.112.3", "2606:4700::1111", "64:ff9b::1.1.1.1"} {
		if internal(netip.MustParseAddr(s)) {
			t.Errorf("%s shouldn't be internal", s)
		}
	}
}

func TestFloor(t *testing.T) {
	pol = writePolicy(t, "**\n10.9.8.7\n")
	if _, err := resolve(t.Context(), "127.0.0.1", 443); !errors.Is(err, errFloor) {
		t.Errorf("loopback: got %v, want the floor", err)
	}
	if _, err := resolve(t.Context(), "10.9.8.6", 443); !errors.Is(err, errFloor) {
		t.Errorf("private: got %v, want the floor", err)
	}
	if a, err := resolve(t.Context(), "10.9.8.7", 443); err != nil || len(a) != 1 {
		t.Errorf("an address rule should pass the floor: %v %v", a, err)
	}
	if _, why := check(t.Context(), "localhost", 443); why == "" {
		t.Error("localhost passed the floor through '**'")
	}
}

// A deny beats any allow, for addresses too: a name that resolves to a denied address isn't dialed,
// even when a host rule or a wider address rule allows it.
func TestAddressDenyAppliesToNames(t *testing.T) {
	pol = writePolicy(t, "localhost:8080\n127.0.0.0/8:8080\n::1:8080\n!127.0.0.1:8080\n!::1:8080\n")
	if ok, _ := pol.decide("127.0.0.1", 8080); ok {
		t.Fatal("the denied address itself is allowed")
	}
	if pol.addrAllowed(netip.MustParseAddr("127.0.0.1"), 8080) {
		t.Error("addrAllowed ignores the deny")
	}
	if _, why := check(t.Context(), "localhost", 8080); why == "" {
		t.Error("localhost, which resolves to denied addresses, passed")
	}
	pol = writePolicy(t, "**\n!1.1.1.1\n")
	if a, err := resolve(t.Context(), "1.1.1.1", 443); err == nil {
		t.Errorf("a denied public address resolved: %v", a)
	}
	// The NAT64 form of a denied IPv4 address (64:ff9b::a.b.c.d, which DNS64 hands out on IPv6-only
	// networks) is denied too, as the floor judges it by the IPv4 address in it.
	pol = writePolicy(t, "**\n!203.0.113.0/24\n")
	n := netip.MustParseAddr("64:ff9b::cb00:7105")
	if !pol.addrDenied(n, 443) {
		t.Error("the NAT64 form of a denied address isn't denied")
	}
	if a, err := resolve(t.Context(), n.String(), 443); err == nil {
		t.Errorf("the NAT64 form of a denied address resolved: %v", a)
	}
}

// With a password, every request needs it (user devvm): CONNECT, absolute-form and devvm.proxy alike.
func TestProxyAuth(t *testing.T) {
	pol = writePolicy(t, "")
	proxyAuth = "k3y"
	t.Cleanup(func() { proxyAuth = "" })
	proxy := httptest.NewServer(http.HandlerFunc(handler))
	defer proxy.Close()
	basic := func(s string) string { return "Basic " + base64.StdEncoding.EncodeToString([]byte(s)) }
	for _, c := range []struct {
		auth string
		want int
	}{
		{"", http.StatusProxyAuthRequired},
		{basic("devvm:wrong"), http.StatusProxyAuthRequired},
		{basic("other:k3y"), http.StatusProxyAuthRequired},
		{basic("devvm:k3y"), http.StatusOK},
	} {
		for _, req := range []string{"GET http://devvm.proxy/blocked HTTP/1.1\r\nHost: devvm.proxy\r\n", "CONNECT devvm.proxy:443 HTTP/1.1\r\nHost: devvm.proxy:443\r\n"} {
			h := req
			if c.auth != "" {
				h += "Proxy-Authorization: " + c.auth + "\r\n"
			}
			got, _ := roundTrip(t, proxy.Listener.Addr().String(), h+"\r\n")
			if strings.HasPrefix(req, "CONNECT") && c.want == http.StatusOK {
				// Past the password: refused by the (empty) policy instead.
				if got == http.StatusProxyAuthRequired {
					t.Errorf("CONNECT with the password got 407")
				}
				continue
			}
			if got != c.want {
				t.Errorf("%s with %q: %d, want %d", strings.Fields(req)[0], c.auth, got, c.want)
			}
		}
	}
}

// Hosts are names or addresses; anything else (shell characters, spaces) is refused before the
// policy, the log or the suggested allow command see it.
func TestOddHostsRefused(t *testing.T) {
	pol = writePolicy(t, "")
	logOut = io.Discard
	proxy := httptest.NewServer(http.HandlerFunc(handler))
	defer proxy.Close()
	for _, req := range []string{
		"CONNECT x;sh*:443 HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://x;sh*/ HTTP/1.1\r\nHost: x;sh*\r\n\r\n",
		"CONNECT $(id):443 HTTP/1.1\r\nHost: x\r\n\r\n",
		"CONNECT [fe80::1%25$(id)]:443 HTTP/1.1\r\nHost: x\r\n\r\n",
	} {
		code, body := roundTrip(t, proxy.Listener.Addr().String(), req)
		if code != http.StatusBadRequest || strings.Contains(body, "policy allow") {
			t.Errorf("%q: %d %q (want 400, no allow command)", strings.SplitN(req, "\r\n", 2)[0], code, body)
		}
	}
	for _, ok := range []string{"example.com", "a-b.c_d.test", "10.0.0.1", "[fd00::1]", "localhost"} {
		if !hostOK(ok) {
			t.Errorf("hostOK(%q) = false", ok)
		}
	}
}

func TestAllowRule(t *testing.T) {
	for _, c := range []struct {
		host string
		port int
		want string
	}{
		{"a.test", 443, "a.test"},
		{"a.test", 80, "a.test"},
		{"db.test", 5432, "db.test:5432"},
		{"fd00::1", 22, "[fd00::1]:22"},
		{"x;sh*", 443, ""},
		{"A.test", 443, ""},
		{"a.test", 0, ""},
		{"a.test", 65536, ""},
	} {
		got, ok := AllowRule(c.host, c.port)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("AllowRule(%q, %d) = %q, %v; want %q", c.host, c.port, got, ok, c.want)
		}
	}
}

// roundTrip sends a raw request to the proxy at addr and returns the response's status and body.
func roundTrip(t *testing.T, addr, req string) (int, string) {
	t.Helper()
	c, err := new(net.Dialer).DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func clientHello(t *testing.T, name string) []byte {
	t.Helper()
	a, b := net.Pipe()
	//nolint:gosec // a test client; it never gets far enough to verify anything
	c := tls.Client(a, &tls.Config{ServerName: name, InsecureSkipVerify: name == ""})
	go func() { _ = c.HandshakeContext(t.Context()) }()
	r := bufio.NewReader(b)
	must(t, b.SetReadDeadline(time.Now().Add(5*time.Second)))
	rec, sni, isTLS := peekClientHello(r)
	_ = a.Close()
	if !isTLS {
		t.Fatal("not recognised as TLS")
	}
	if sni != name {
		t.Errorf("sni = %q, want %q", sni, name)
	}
	return rec
}

func TestClientHello(t *testing.T) {
	if rec := clientHello(t, "api.example.com"); len(rec) < 50 || rec[0] != 0x16 {
		t.Errorf("record not returned for replay: %d bytes", len(rec))
	}
	clientHello(t, "")
	if _, _, isTLS := peekClientHello(bufio.NewReader(bytes.NewReader([]byte("SSH-2.0-OpenSSH_9.9\r\n")))); isTLS {
		t.Error("SSH treated as TLS")
	}
}

func TestSNIMatches(t *testing.T) {
	if !sniMatches("API.example.com", "api.example.com") || sniMatches("other.com", "api.example.com") {
		t.Error("name check")
	}
	if !sniMatches("", "1.2.3.4") || sniMatches("", "example.com") {
		t.Error("empty SNI is only fine for address literals")
	}
}

// Main: -help prints the flags, bad arguments and a missing or empty password are errors, and
// nothing is served then.
func TestMainRefuses(t *testing.T) {
	var out, errs bytes.Buffer
	std := IO{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errs}
	if err := Main([]string{"--help"}, std); err != nil || !strings.Contains(out.String(), "-auth-file") {
		t.Errorf("--help: %v, %q", err, out.String())
	}
	empty := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(empty, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-nope"},
		{"extra"},
		{},
		{"-auth-file", filepath.Join(t.TempDir(), "missing")},
		{"-auth-file", empty},
	} {
		if err := Main(args, std); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}

// A short run: the proxy listens, logs that it does to stderr, answers with the VM's password, and
// returns once the VM's hostagent (here a sleep) is gone.
func TestMainRuns(t *testing.T) {
	lifelineTick = 50 * time.Millisecond
	t.Cleanup(func() { lifelineTick = 3 * time.Second; proxyAuth = "" })
	dir := t.TempDir()
	key, pidFile, logFile := filepath.Join(dir, "key"), filepath.Join(dir, "ha.pid"), filepath.Join(dir, "log.jsonl")
	if err := os.WriteFile(key, []byte("k3y\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sleep := exec.CommandContext(t.Context(), "sleep", "60")
	if err := sleep.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleep.Process.Kill() })
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(sleep.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	// A free port, given back for the proxy to take.
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	errs := &syncBuffer{}
	done := make(chan error)
	go func() {
		done <- Main([]string{
			"--listen", addr, "-vm", "devvm-x", "-auth-file", key, "-lifeline", pidFile, "-log", logFile,
			"-policy", filepath.Join(dir, "missing"),
		}, IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: errs})
	}()
	pu, _ := url.Parse("http://devvm:k3y@" + addr)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(pu)}}
	fetch := func(u string) (int, string, error) {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, u, nil)
		resp, err := client.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), nil
	}
	var code int
	var body string
	for range 100 {
		if code, body, err = fetch("http://devvm.proxy/blocked"); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if code != http.StatusOK || body == "" {
		t.Errorf("/blocked: %d %q", code, body)
	}
	if code, _, err := fetch("http://blocked.test/"); err != nil || code != http.StatusForbidden {
		t.Errorf("blocked.test: %d %v", code, err)
	}
	b, _ := os.ReadFile(logFile) //nolint:gosec // the test's own file
	if !strings.Contains(string(b), `"vm":"devvm-x","event":"blocked","host":"blocked.test"`) {
		t.Errorf("event log = %q", b)
	}
	time.Sleep(5 * lifelineTick) // the proxy has seen the hostagent
	_ = sleep.Process.Kill()
	_ = sleep.Wait()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Main: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("still running after the hostagent was gone: %s", errs.String())
	}
	if s := errs.String(); !strings.Contains(s, "devvm proxy: ") || !strings.Contains(s, "listening on "+addr+" for devvm-x") ||
		!strings.Contains(s, "hostagent is gone") {
		t.Errorf("stderr = %q", s)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// The rule syntax: whatever a policy file holds, parsing doesn't panic, a rule's text parses
// back to the same rule, and a host rule never holds a space or a slash.
func FuzzParseRule(f *testing.F) {
	for _, s := range []string{
		"example.com", "*.github.com  # c", "github.com:22", "!evil.github.com", "10.1.2.3",
		"192.168.0.0/16:8080", "**:9999", "[::1]:8080", "::1:8080", "!127.0.0.1:8080", "fd00::/8", "a b", "x/y", "#", ":", "!",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		r, ok := parseRule(line)
		if !ok {
			return
		}
		again, ok := parseRule(r.text)
		if !ok || again != r {
			t.Fatalf("%q: %+v, parsed again %+v (%v)", line, r, again, ok)
		}
		if strings.ContainsAny(r.host, " /\\#") {
			t.Fatalf("%q: host %q", line, r.host)
		}
		_ = r.matchHost("example.com")
		_ = r.matchAddr(netip.MustParseAddr("64:ff9b::a00:1"))
	})
}

// A request's target, as the handler reads it: one the proxy accepts is a plain host (no shell
// characters, for the log and the allow command it suggests), and that allow command's rule
// allows exactly that host and port.
func FuzzRequest(f *testing.F) {
	for _, s := range []string{
		"CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n",
		"CONNECT [fd00::1]:22 HTTP/1.1\r\nHost: x\r\n\r\n",
		"CONNECT x;sh*:443 HTTP/1.1\r\nHost: x\r\n\r\n",
		"CONNECT [fe80::1%25$(id)]:443 HTTP/1.1\r\nHost: x\r\n\r\n",
		"GET http://Example.COM.:8080/x HTTP/1.1\r\nHost: example.com\r\n\r\n",
		"GET https://10.0.0.1/ HTTP/1.1\r\nHost: 10.0.0.1\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: devvm.proxy\r\n\r\n",
	} {
		f.Add(s)
	}
	dir := f.TempDir()
	n := 0
	f.Fuzz(func(t *testing.T, raw string) {
		r, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
		if err != nil {
			return
		}
		var host string
		var port int
		if r.Method == http.MethodConnect {
			host, port = splitHostPort(r.Host, 443)
		} else {
			def := 80
			if r.URL.Scheme == "https" {
				def = 443
			}
			host, port = splitHostPort(r.URL.Host, def)
		}
		if !hostOK(host) {
			return
		}
		for _, c := range host {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && !strings.ContainsRune("-_.:[]", c) {
				t.Fatalf("%q: host %q holds %q", raw, host, c)
			}
		}
		if port < 1 || port > 65535 {
			return // never allowed: no rule names such a port
		}
		n++
		file := filepath.Join(dir, strconv.Itoa(n))
		if err := os.WriteFile(file, []byte(ruleFor(host, port)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if ok, why := newPolicy([]string{file}).decide(host, port); !ok {
			t.Fatalf("%q: the rule %q doesn't allow %s port %d: %s", raw, ruleFor(host, port), host, port, why)
		}
	})
}

// A tunnel's first bytes: whatever they are, reading a ClientHello doesn't panic or hang, and
// only a TLS handshake record counts as TLS.
func FuzzClientHello(f *testing.F) {
	f.Add([]byte("SSH-2.0-OpenSSH_9.9\r\n"))
	f.Add([]byte{0x16, 3, 1, 0, 4, 1, 0, 0, 0})
	f.Add([]byte{0x16, 3, 1, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, b []byte) {
		rec, _, isTLS := peekClientHello(bufio.NewReader(bytes.NewReader(b)))
		if isTLS != (len(b) >= 5 && b[0] == 0x16 && b[1] == 3) {
			t.Fatalf("% x: TLS = %v", b, isTLS)
		}
		if !isTLS && rec != nil {
			t.Fatalf("% x: a record from a stream that isn't TLS", b)
		}
	})
}
