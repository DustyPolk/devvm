package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeAtomic replaces a file whole with the mode asked for, a file there
// with another mode included, and leaves nothing else; one it can't write
// leaves nothing either.
func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ca.crt")
	for _, tc := range []struct {
		data string
		mode os.FileMode
	}{{"one\n", 0o644}, {"two\n", 0o600}, {"three\n", 0o644}} {
		if err := writeAtomic(path, []byte(tc.data), tc.mode); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path) //nolint:gosec // the test's own file
		fi, serr := os.Stat(path)
		if err != nil || serr != nil || string(b) != tc.data || fi.Mode().Perm() != tc.mode {
			t.Errorf("%q, %v: %v, %v", b, fi.Mode(), err, serr)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(dir, "sub"), []byte("x"), 0o600); err == nil {
		t.Error("a folder replaced")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 2 {
		t.Errorf("left %v", ents)
	}
}

// writeBindings writes the bindings file at path as devvm does.
func writeBindings(t *testing.T, path string, bs ...Binding) {
	t.Helper()
	must(t, os.WriteFile(path, BindingList{Bindings: bs}.Encode(), 0o600))
}

// bound is the secret t bound to hosts, in Authorization, for the placeholder PH.
func bound(hosts ...string) Binding {
	return Binding{Name: "t", Kind: KindSecret, Hosts: hosts, Header: "Authorization", Placeholder: "PH"}
}

// fakeSecret makes every lookup give value, with nothing cached yet, and returns the bindings
// looked up, in order.
func fakeSecret(t *testing.T, value string) *[]Binding {
	t.Helper()
	var asked []Binding
	old := secretLookup
	secretLookup = func(_ context.Context, b Binding) (string, error) { asked = append(asked, b); return value, nil }
	values = map[string]cached{}
	t.Cleanup(func() { secretLookup, values = old, map[string]cached{} })
	return &asked
}

func TestInject(t *testing.T) {
	fakeSecret(t, "real")
	bs := []Binding{{Name: "github", Kind: KindSecret, Header: "Authorization", Placeholder: "PH"}}
	h := http.Header{}
	h.Set("Authorization", "token PH")
	h.Set("X-Echo", "PH")
	if used := inject(h, bs); len(used) != 1 || used[0] != "github" {
		t.Errorf("used = %v", used)
	}
	if h.Get("Authorization") != "token real" || h.Get("X-Echo") != "PH" {
		t.Errorf("headers = %v (only the binding's header may change)", h)
	}
	h = http.Header{}
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:PH")))
	inject(h, bs)
	if raw, _ := basic(h.Get("Authorization")); raw != "x-access-token:real" {
		t.Errorf("basic = %q", raw)
	}
	h = http.Header{}
	h.Set("Authorization", "Bearer something-else")
	if used := inject(h, bs); len(used) != 0 || h.Get("Authorization") != "Bearer something-else" {
		t.Error("a header without the placeholder changed")
	}
}

// interceptSetup: devvm's CA in a temp dir, an upstream HTTPS server for "localhost" (handler h) with a
// certificate from it, a policy allowing it, a secret bound to it (placeholder PH, value "real"), and
// the proxy. It returns the upstream's port, the bindings file, and a client that goes through the
// proxy and trusts the CA, keeping one connection open.
func interceptSetup(t *testing.T, h http.HandlerFunc) (int, string, *http.Client) {
	t.Helper()
	dir := t.TempDir()
	writeCA(t, dir)
	if err := loadCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")); err != nil {
		t.Fatal(err)
	}
	leaves = map[string]*tls.Certificate{} // minted by another test's CA
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	upstreamCAs = pool
	t.Cleanup(func() { upstreamCAs = nil })
	up := httptest.NewUnstartedServer(h)
	cert, _ := leafFor("localhost")
	up.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	up.StartTLS()
	t.Cleanup(up.Close)
	_, portStr, _ := net.SplitHostPort(up.Listener.Addr().String())
	port, err := strconv.Atoi(portStr)
	must(t, err)
	interceptPorts[port] = true
	t.Cleanup(func() { delete(interceptPorts, port) })
	pol = writePolicy(t, fmt.Sprintf("localhost:%d\n127.0.0.1:%d\n", port, port))
	secretsPath = filepath.Join(dir, "bindings.json")
	writeBindings(t, secretsPath, bound("localhost"))
	t.Cleanup(func() { secretsPath = "" })
	fakeSecret(t, "real")
	proxy := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(proxy.Close)
	pu, _ := url.Parse(proxy.URL)
	return port, secretsPath, &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: pool}, MaxConnsPerHost: 1,
	}}
}

func get(t *testing.T, c *http.Client, url, host string) (int, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer PH")
	if host != "" {
		req.Host = host
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// A secret goes only to the host it's bound to: inside an intercepted tunnel, a request whose Host
// names another site (one sharing the bound host's CDN, say) is refused, not forwarded with the token.
func TestInterceptRefusesAnotherHost(t *testing.T) {
	port, _, c := interceptSetup(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s", r.Host, r.Header.Get("Authorization")) //nolint:gosec // a test echo server
	})
	u := fmt.Sprintf("https://localhost:%d/x", port)
	if code, body := get(t, c, u, ""); code != 200 || body != fmt.Sprintf("localhost:%d|Bearer real", port) {
		t.Fatalf("same host: %d %q", code, body)
	}
	code, body := get(t, c, u, "other.example")
	if code != http.StatusMisdirectedRequest || strings.Contains(body, "real") {
		t.Errorf("another Host: %d %q (want 421 and no secret)", code, body)
	}
}

// Unbinding a secret applies to tunnels already open: the next request in a kept-alive tunnel gets
// no secret.
func TestUnbindAppliesToOpenTunnels(t *testing.T) {
	port, file, c := interceptSetup(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("Authorization")) //nolint:gosec // a test echo server
	})
	u := fmt.Sprintf("https://localhost:%d/x", port)
	if _, body := get(t, c, u, ""); body != "Bearer real" {
		t.Fatalf("bound: %q", body)
	}
	writeBindings(t, file)
	later := time.Now().Add(2 * time.Second)
	must(t, os.Chtimes(file, later, later))
	if _, body := get(t, c, u, ""); body != "Bearer PH" {
		t.Errorf("after unbinding, the open tunnel still got %q", body)
	}
}

// writeCA writes a CA into dir as devvm's (trust.InitCA) is: ca.pem, and its ECDSA key, ca.key,
// "EC PRIVATE KEY", mode 600. The proxy's tests can't import trust, which isn't the standard library.
func writeCA(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "devvm proxy CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(30 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, BasicConstraintsValid: true, IsCA: true, MaxPathLenZero: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(t, err)
	kder, err := x509.MarshalECPrivateKey(key)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, "ca.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)) //nolint:gosec // a certificate
}

// Values from agents' login files are cached briefly: a logout on the host stops them soon. The
// binding's kind decides, never its name.
func TestLoginValuesCacheBriefly(t *testing.T) {
	for _, tc := range []struct {
		b    Binding
		want time.Duration
	}{
		{Binding{Name: "claude", Kind: KindSecret}, valueTTL},
		{Binding{Name: "github", Kind: KindSecret}, valueTTL},
		{Binding{Name: "codex", Kind: KindLogin, Agent: "codex"}, loginTTL},
		{Binding{Name: "opencode-anthropic", Kind: KindLogin, Agent: "opencode"}, loginTTL},
		{Binding{Name: "claude-mcp-abc", Kind: KindLogin, Agent: "claude"}, loginTTL},
		{Binding{Name: "codex-key", Kind: KindSecret}, valueTTL},
	} {
		if got := ttlOf(tc.b); got != tc.want {
			t.Errorf("ttlOf(%+v) = %v, want %v", tc.b, got, tc.want)
		}
	}
}

// A value is looked up in this process, with the binding the file has, once, then cached until its
// time is up; a lookup that fails, or gives nothing, isn't cached, and puts nothing in.
func TestSecretValueLooksUpInProcess(t *testing.T) {
	asked := fakeSecret(t, " real\n")
	b := Binding{Name: "codex", Kind: KindLogin, Agent: "codex"}
	for range 2 {
		if v, err := secretValue(b); v != "real" || err != nil {
			t.Fatalf("secretValue = %q, %v", v, err)
		}
	}
	if len(*asked) != 1 || !reflect.DeepEqual((*asked)[0], b) {
		t.Errorf("looked up %+v, want %+v once", *asked, b)
	}
	values[b.Name] = cached{"real", time.Now().Add(-loginTTL)}
	if _, err := secretValue(b); err != nil || len(*asked) != 2 {
		t.Errorf("an expired value: %v, %d lookups", err, len(*asked))
	}
	values = map[string]cached{}
	secretLookup = func(context.Context, Binding) (string, error) { return "", errors.New("locked") }
	if _, err := secretValue(b); err == nil || !strings.Contains(err.Error(), "codex: locked") {
		t.Errorf("a failed lookup: %v", err)
	}
	secretLookup = func(context.Context, Binding) (string, error) { return " ", nil }
	if _, err := secretValue(b); err == nil {
		t.Error("an empty value was taken")
	}
	if _, ok := values[b.Name]; ok {
		t.Error("a failed lookup was cached")
	}
	secretLookup = nil
	if _, err := secretValue(b); err == nil {
		t.Error("a value without a lookup")
	}
	h := http.Header{}
	h.Set("Authorization", "Bearer PH")
	if used := inject(h, []Binding{{Name: "codex", Kind: KindLogin, Header: "Authorization", Placeholder: "PH"}}); used != nil || h.Get("Authorization") != "Bearer PH" {
		t.Errorf("without a value: used %v, %q", used, h.Get("Authorization"))
	}
}

// The bindings file: versioned, private, of known kinds; InterceptLines gives its hosts on 443.
func TestReadBindings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	l := BindingList{Bindings: []Binding{
		{Name: "github", Kind: KindSecret, Hosts: []string{"api.github.com"}, Header: "Authorization", Placeholder: "PH"},
		{Name: "codex", Kind: KindLogin, Agent: "codex", Hosts: []string{"chatgpt.com"}, Header: "Authorization", Placeholder: "PH2"},
	}}
	must(t, os.WriteFile(path, l.Encode(), 0o600))
	got, err := ReadBindings(path)
	l.Version = BindingListVersion
	if err != nil || !reflect.DeepEqual(got, l) {
		t.Errorf("ReadBindings = %+v, %v; want %+v", got, err, l)
	}
	if s := string(BindingList{}.Encode()); s != `{"version":1,"bindings":[]}`+"\n" {
		t.Errorf("no bindings = %q", s)
	}
	for name, tc := range map[string]struct {
		data string
		mode os.FileMode
	}{
		"another version": {`{"version":2,"bindings":[]}`, 0o600},
		"a bare list":     {`[]`, 0o600},
		"an unknown kind": {`{"version":1,"bindings":[{"name":"x","kind":"env"}]}`, 0o600},
		"no kind":         {`{"version":1,"bindings":[{"name":"x"}]}`, 0o600},
		"others may read": {`{"version":1,"bindings":[]}`, 0o644},
	} {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		must(t, os.WriteFile(p, []byte(tc.data), 0o600))
		must(t, os.Chmod(p, tc.mode))
		if _, err := ReadBindings(p); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	link := filepath.Join(dir, "link")
	must(t, os.Symlink(path, link))
	if _, err := ReadBindings(link); !errors.Is(err, ErrNotRegular) {
		t.Errorf("a link: %v", err)
	}
	hosts := InterceptLines([]string{"api.github.com", "github.com", "api.github.com"})
	if want := []string{"api.github.com:443", "github.com:443"}; !reflect.DeepEqual(hosts, want) {
		t.Errorf("InterceptLines = %q, want %q", hosts, want)
	}
	if InterceptLines(nil) != nil {
		t.Error("InterceptLines(nil) isn't empty")
	}
}

// A bindings file the proxy can't read, or that's gone, has no bindings: nothing goes in.
func TestBindingsUnreadableHaveNone(t *testing.T) {
	secretsPath = filepath.Join(t.TempDir(), "bindings.json")
	t.Cleanup(func() { secretsPath = "" })
	must(t, os.WriteFile(secretsPath, BindingList{Bindings: []Binding{{Name: "t", Kind: KindSecret, Hosts: []string{"localhost"}}}}.Encode(), 0o600))
	if bs := bindingsFor("localhost"); len(bs) != 1 {
		t.Fatalf("bindings = %+v", bs)
	}
	must(t, os.Chmod(secretsPath, 0o644)) //nolint:gosec // G302: the file others may read is what's tested
	later := time.Now().Add(2 * time.Second)
	must(t, os.Chtimes(secretsPath, later, later))
	if bs := bindingsFor("localhost"); bs != nil {
		t.Errorf("a file others may read gave %+v", bs)
	}
	must(t, os.Remove(secretsPath))
	if bs := bindingsFor("localhost"); bs != nil {
		t.Errorf("a file that's gone gave %+v", bs)
	}
}

// useCA makes the proxy's CA one valid from notBefore to notAfter, for this test, with no leaves
// minted yet.
func useCA(t *testing.T, notBefore, notAfter time.Time) {
	t.Helper()
	oldCert, oldKey, oldLeafKey, oldLeaves := caCert, caKey, leafKey, leaves
	t.Cleanup(func() { caCert, caKey, leafKey, leaves = oldCert, oldKey, oldLeafKey, oldLeaves })
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: notBefore, NotAfter: notAfter,
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(t, err)
	caCert, err = x509.ParseCertificate(der)
	must(t, err)
	caKey = key
	leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(t, err)
	leaves = map[string]*tls.Certificate{}
}

// A leaf never outlives the CA that signed it, or starts before it: a VM left running near the CA's
// end gets leaves that end with it, kept until then, and none once it has ended.
func TestLeafWithinCA(t *testing.T) {
	start, end := time.Now().Add(-time.Minute).Truncate(time.Second), time.Now().Add(30*time.Minute).Truncate(time.Second)
	useCA(t, start, end)
	c, err := leafFor("api.example.com")
	must(t, err)
	if c.Leaf.NotAfter.After(end) || c.Leaf.NotBefore.Before(start) {
		t.Errorf("a leaf valid %v to %v, from a CA valid %v to %v", c.Leaf.NotBefore, c.Leaf.NotAfter, start, end)
	}
	if again, err := leafFor("api.example.com"); err != nil || again != c {
		t.Errorf("a leaf that ends with its CA was minted again (%v)", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := c.Leaf.Verify(x509.VerifyOptions{DNSName: "api.example.com", Roots: pool}); err != nil {
		t.Errorf("the leaf doesn't verify: %v", err)
	}

	useCA(t, time.Now().Add(-30*24*time.Hour), time.Now().Add(-time.Minute))
	if c, err := leafFor("api.example.com"); !errors.Is(err, errCAEnded) || c != nil {
		t.Errorf("a leaf from a CA that has ended: %v, %v", c != nil, err)
	}
}

// A tunnel to a host with a bound secret, once the proxy's CA has ended, is refused before it
// opens: 403 with the reason and no allow command, logged as a refused block.
func TestInterceptCAEnded(t *testing.T) {
	port, _, _ := interceptSetup(t, func(http.ResponseWriter, *http.Request) {})
	var events strings.Builder
	oldOut := logOut
	logOut = &events
	t.Cleanup(func() { logOut = oldOut })
	useCA(t, time.Now().Add(-30*24*time.Hour), time.Now().Add(-time.Minute))
	target := net.JoinHostPort("localhost", strconv.Itoa(port))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodConnect, "http://"+target, nil)
	req.Host = target
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), caEndedReason) ||
		strings.Contains(w.Body.String(), "network allow") || w.Header().Get("X-Devvm-Blocked") != "localhost" {
		t.Errorf("answered %d %q", w.Code, w.Body.String())
	}
	var e Event
	must(t, json.Unmarshal([]byte(events.String()), &e))
	if e.Event != "blocked" || e.Reason != caEndedReason || !e.Refused || e.Host != "localhost" || e.Port != port {
		t.Errorf("logged %+v", e)
	}
}

// devvm touches the bindings file when a value is set or removed: cached values are dropped.
func TestBindingsChangeDropsCachedValues(t *testing.T) {
	secretsPath = filepath.Join(t.TempDir(), "bindings.json")
	t.Cleanup(func() { secretsPath = "" })
	writeBindings(t, secretsPath)
	bindings.all()
	valueMu.Lock()
	values["t"] = cached{"old", time.Now()}
	valueMu.Unlock()
	later := time.Now().Add(2 * time.Second)
	must(t, os.Chtimes(secretsPath, later, later))
	bindings.all()
	valueMu.Lock()
	defer valueMu.Unlock()
	if _, ok := values["t"]; ok {
		t.Error("a cached value survived the bindings file changing")
	}
}

// An intercepted HTTPS request through the proxy: the client trusts only devvm's CA and sends the
// placeholder; the upstream server sees the real secret.
func TestIntercept(t *testing.T) {
	dir := t.TempDir()
	writeCA(t, dir)
	if err := loadCA(filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")); err != nil {
		t.Fatal(err)
	}
	leaves = map[string]*tls.Certificate{} // minted by another test's CA
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	upstreamCAs = pool
	t.Cleanup(func() { upstreamCAs = nil })

	// Upstream: echoes what it received, with a certificate for "localhost" from the same CA.
	up := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s|%s|%s", r.Header.Get("Authorization"), r.Header.Get("X-Echo"), r.Host) //nolint:gosec // a test echo server
	}))
	cert, _ := leafFor("localhost")
	up.TLS = &tls.Config{Certificates: []tls.Certificate{*cert}}
	up.StartTLS()
	defer up.Close()
	_, portStr, _ := net.SplitHostPort(up.Listener.Addr().String())
	port, err := strconv.Atoi(portStr)
	must(t, err)
	interceptPorts[port] = true
	t.Cleanup(func() { delete(interceptPorts, port) })

	pol = writePolicy(t, fmt.Sprintf("localhost:%d\n127.0.0.1:%d\n", port, port))
	secretsPath = filepath.Join(dir, "bindings.json")
	writeBindings(t, secretsPath, bound("localhost"))
	t.Cleanup(func() { secretsPath = "" })
	fakeSecret(t, "real-secret")

	proxy := httptest.NewServer(http.HandlerFunc(handler))
	defer proxy.Close()
	pu, _ := url.Parse(proxy.URL)
	client := &http.Client{Transport: &http.Transport{
		Proxy:           http.ProxyURL(pu),
		TLSClientConfig: &tls.Config{RootCAs: pool},
	}}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("https://localhost:%d/x", port), nil)
	req.Header.Set("Authorization", "Bearer PH")
	req.Header.Set("X-Echo", "PH")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if got, want := string(body), fmt.Sprintf("Bearer real-secret|PH|localhost:%d", port); got != want {
		t.Errorf("upstream saw %q, want %q", got, want)
	}
	// The Host upstream gets is the one checked, canonical: not the VM's spelling (a trailing dot).
	req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("https://localhost.:%d/x", port), nil)
	req.Header.Set("Authorization", "Bearer PH")
	if resp, err := client.Do(req); err != nil {
		t.Error(err)
	} else {
		body, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if got := string(body); !strings.HasSuffix(got, fmt.Sprintf("|localhost:%d", port)) {
			t.Errorf("upstream saw Host %q", got)
		}
	}

	// The CA is served to the VM; the private key never is.
	req, _ = http.NewRequestWithContext(t.Context(), http.MethodGet, "http://devvm.proxy/ca.pem", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BEGIN CERTIFICATE") || strings.Contains(string(body), "PRIVATE") {
		t.Errorf("/ca.pem = %q", body)
	}
}

// Calls that make lasting GitHub credentials with the VM's token are refused; ordinary ones aren't.
func TestMintsCredential(t *testing.T) {
	for _, c := range []struct{ host, method, path, want string }{
		{"api.github.com", "POST", "/user/keys", "a key on your GitHub account"},
		{"api.github.com", "post", "/User/SSH_Signing_Keys/", "a key on your GitHub account"},
		{"api.github.com", "POST", "/repos/o/r/keys", "a deploy key"},
		{"api.github.com", "POST", "//repos/o/r/./keys", "a deploy key"},
		{"api.github.com", "POST", "/repos/o/r/x/../keys", "a deploy key"},
		{"api.github.com", "POST", "/orgs/o/actions/runners/registration-token", "a self-hosted runner"},
		{"api.github.com", "POST", "/repos/o/r/actions/runners/generate-jitconfig", "a self-hosted runner"},
		{"api.github.com", "PATCH", "/repos/o/r/hooks/1/config", "a webhook"},
		{"api.github.com", "POST", "/orgs/o/hooks", "a webhook"},
		{"api.github.com", "GET", "/user/keys", ""},
		{"api.github.com", "DELETE", "/repos/o/r/keys/1", ""},
		{"api.github.com", "POST", "/repos/o/r/pulls", ""},
		{"api.github.com", "POST", "/repos/o/r/issues/1/comments", ""},
		{"github.com", "POST", "/user/keys", ""},
	} {
		if got := mintsCredential(c.host, c.method, c.path); got != c.want {
			t.Errorf("%s %s%s = %q, want %q", c.method, c.host, c.path, got, c.want)
		}
	}
}
