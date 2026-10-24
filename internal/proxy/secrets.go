package proxy

// Secrets: the VM only ever holds placeholders. For hosts with a bound secret, the proxy terminates
// TLS with a certificate from devvm's own CA (trusted only inside VMs), replaces the placeholder in
// the binding's header with the real value, and forwards over a fresh TLS connection. Values come
// from `devvm secret get NAME` (the macOS Keychain) on demand and are cached briefly; they're never
// logged or written anywhere.

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
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// binding: which hosts get a secret, in which header, and the placeholder the VM sends instead.
type binding struct {
	Name        string   `json:"name"`
	Hosts       []string `json:"hosts"`  // host patterns, as in policy files
	Header      string   `json:"header"` // e.g. Authorization; the placeholder is replaced inside its value
	Placeholder string   `json:"placeholder"`
}

var (
	secretsPath = ""           // -secrets: JSON list of bindings (no values), re-read on change
	secretCmd   = ""           // -secret-cmd: the devvm script, run as `devvm secret get NAME`
	upstreamCAs *x509.CertPool // nil: the system roots (tests set their own)
	// Only HTTPS is intercepted; anything else to a bound host (SSH on :22, …) is tunneled.
	interceptPorts = map[int]bool{443: true}
)

type bindingFile struct {
	mu    sync.Mutex
	mtime time.Time
	list  []binding
}

var bindings = &bindingFile{}

func (b *bindingFile) all() []binding {
	b.mu.Lock()
	defer b.mu.Unlock()
	if secretsPath == "" {
		return nil
	}
	st, err := os.Stat(secretsPath)
	if err != nil {
		b.list = nil
		return nil
	}
	if !st.ModTime().Equal(b.mtime) {
		var list []binding
		if data, err := os.ReadFile(secretsPath); err == nil && json.Unmarshal(data, &list) == nil {
			b.list, b.mtime = list, st.ModTime()
			logf("secrets %s: %d bindings", secretsPath, len(list))
		}
		// devvm touches the file when a value is set or removed: look values up afresh.
		valueMu.Lock()
		values = map[string]cached{}
		valueMu.Unlock()
	}
	return b.list
}

// bindingsFor: the bindings that apply to host.
func bindingsFor(host string) []binding {
	var out []binding
	for _, b := range bindings.all() {
		for _, h := range b.Hosts {
			if r, ok := parseRule(h); ok && !r.deny && r.matchHost(strings.ToLower(host)) {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

// --- values ---

type cached struct {
	value string
	at    time.Time
}

var (
	valueMu sync.Mutex
	values  = map[string]cached{}
)

// A value is cached for valueTTL; one from an agent's login file (Codex, opencode, Claude MCP) for
// loginTTL, since logging out on the host changes nothing devvm sees.
const (
	valueTTL = 5 * time.Minute
	loginTTL = 30 * time.Second
)

func ttlOf(name string) time.Duration {
	if name == "codex" || name == "codex-key" || strings.HasPrefix(name, "opencode-") || strings.HasPrefix(name, "claude-mcp-") {
		return loginTTL
	}
	return valueTTL
}

// secretValue runs `devvm secret get NAME` (cached, see ttlOf).
var secretValue = func(name string) (string, error) {
	valueMu.Lock()
	defer valueMu.Unlock()
	if c, ok := values[name]; ok && time.Since(c.at) < ttlOf(name) {
		return c.value, nil
	}
	if secretCmd == "" {
		return "", errors.New("no -secret-cmd")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, secretCmd, "secret", "get", name).Output() //nolint:gosec // devvm names itself
	if err != nil {
		return "", fmt.Errorf("devvm secret get %s: %w", name, err)
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", fmt.Errorf("secret %s is empty", name)
	}
	values[name] = cached{v, time.Now()}
	return v, nil
}

// inject replaces each binding's placeholder in its header, also inside "Basic" credentials (git
// over HTTPS sends user:token that way). Only the binding's header is touched. It returns the names
// of the secrets it put in.
func inject(h http.Header, bs []binding) (used []string) {
	for _, b := range bs {
		vs := h.Values(b.Header)
		if len(vs) == 0 {
			continue
		}
		var val string
		var err error
		replaced := false
		for i, v := range vs {
			nv := v
			if strings.Contains(nv, b.Placeholder) || basicHas(nv, b.Placeholder) {
				if val == "" {
					if val, err = secretValue(b.Name); err != nil {
						logf("%v", err)
						return used
					}
				}
				nv = strings.ReplaceAll(nv, b.Placeholder, val)
				if raw, ok := basic(nv); ok && strings.Contains(raw, b.Placeholder) {
					nv = "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(raw, b.Placeholder, val)))
				}
				vs[i], replaced = nv, true
			}
		}
		if replaced {
			h.Del(b.Header)
			for _, v := range vs {
				h.Add(b.Header, v)
			}
			used = append(used, b.Name)
		}
	}
	return used
}

func basic(v string) (string, bool) {
	if len(v) < 6 || !strings.EqualFold(v[:6], "basic ") {
		return "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v[6:]))
	return string(raw), err == nil
}

func basicHas(v, placeholder string) bool {
	raw, ok := basic(v)
	return ok && strings.Contains(raw, placeholder)
}

// --- the CA ---

var (
	caCert  *x509.Certificate
	caKey   *ecdsa.PrivateKey
	leafKey *ecdsa.PrivateKey
	leafMu  sync.Mutex
	leaves  = map[string]*tls.Certificate{}
)

// The CA lasts caLife and is replaced when it has less than caRenew left: a key that leaks (in a
// backup, say) stops working within weeks. VMs trust a new CA from their next launch; a running
// proxy keeps the one it started with, which is still valid then.
const (
	caLife  = 30 * 24 * time.Hour
	caRenew = 7 * 24 * time.Hour
)

// initCA writes a new CA into dir (ca.pem, ca.key mode 600) unless a valid one with over caRenew
// left is there. One valid for longer than caLife (made before CAs were rotated) is replaced too.
func initCA(dir string) error {
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "ca.key")
	if _, err := os.Stat(keyPath); err == nil {
		if pair, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
			if c, err := x509.ParseCertificate(pair.Certificate[0]); err == nil && time.Until(c.NotAfter) > caRenew &&
				c.NotAfter.Sub(c.NotBefore) <= caLife+time.Hour {
				return nil
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		// No host name: every VM sees this certificate.
		Subject:               pkix.Name{CommonName: "devvm proxy CA", Organization: []string{"devvm"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caLife),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// Each file whole or not at all (temp file, then rename), so a crash halfway leaves no pair that
	// doesn't match.
	if err := writeAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder}), 0o600); err != nil {
		return err
	}
	return writeAtomic(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".devvm-ca-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func loadCA(certPath, keyPath string) error {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return err
	}
	k, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return errors.New("the CA key isn't ECDSA")
	}
	if caCert, err = x509.ParseCertificate(pair.Certificate[0]); err != nil {
		return err
	}
	caKey = k
	leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	return err
}

func caPEM() []byte {
	if caCert == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
}

// leafFor mints (and caches for a day) a certificate for host, signed by the CA.
func leafFor(host string) (*tls.Certificate, error) {
	leafMu.Lock()
	defer leafMu.Unlock()
	if c, ok := leaves[host]; ok && time.Until(c.Leaf.NotAfter) > time.Hour {
		return c, nil
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	leaf, _ := x509.ParseCertificate(der)
	c := &tls.Certificate{Certificate: [][]byte{der, caCert.Raw}, PrivateKey: leafKey, Leaf: leaf}
	leaves[host] = c
	return c, nil
}

// --- interception ---

// intercept serves one CONNECT tunnel to host:443 whose host has bindings: TLS with a minted
// certificate on the client side, HTTP/1.1 or HTTP/2 inside, and each request forwarded upstream
// over a new TLS connection with the secrets put in. Each request must be for host itself: the
// secrets are bound to it, so another Host (a site on the same CDN, say) gets 421.
func intercept(client net.Conn, host string, port int, e Event) {
	tconn := tls.Server(client, &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return leafFor(host)
		},
	})
	if err := tconn.HandshakeContext(context.Background()); err != nil {
		e.Event, e.Reason = "blocked", "TLS with the VM failed (is the devvm CA installed?): "+err.Error()
		record(e)
		_ = client.Close()
		return
	}
	e.Event = "intercept"
	record(e)
	cn := &closeNotifyConn{Conn: tconn, done: make(chan struct{})}
	fwd := forwarder("https", net.JoinHostPort(host, fmt.Sprint(port)), host, port)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h, _ := splitHostPort(r.Host, port); h != host {
				http.Error(w, "devvm proxy: this connection is for "+host, http.StatusMisdirectedRequest)
				return
			}
			if what := mintsCredential(host, r.Method, r.URL.Path); what != "" {
				record(Event{
					Event: "blocked", Host: host, Port: port, Method: r.Method, Path: r.URL.Path, Refused: true,
					Reason: "makes " + what + " with your token, which would outlast the VM; do it on the host",
				})
				http.Error(w, "devvm proxy: refused: this makes "+what+" with your token (do it on the host)", http.StatusForbidden)
				return
			}
			fwd.ServeHTTP(w, r)
		}),
		Protocols:         new(http.Protocols),
		IdleTimeout:       10 * time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
	}
	// TLS is already done here, so the server sees plain HTTP/1.1, or HTTP/2 with prior knowledge.
	srv.Protocols.SetHTTP1(true)
	srv.Protocols.SetUnencryptedHTTP2(true)
	_ = srv.Serve(&oneConnListener{c: cn, done: cn.done})
}

// GitHub API calls that make a credential of their own with the token the proxy puts in: an SSH or
// GPG key on your account, a repo's deploy key, a self-hosted runner's registration, a webhook
// (which sends a repo's events anywhere). Each would keep working after devvm secret unuse.
var githubMinting = []struct {
	methods string
	path    *regexp.Regexp
	what    string
}{
	{"POST", regexp.MustCompile(`^/user/(keys|ssh_signing_keys|gpg_keys)$`), "a key on your GitHub account"},
	{"POST", regexp.MustCompile(`^/repos/[^/]+/[^/]+/keys$`), "a deploy key"},
	{"POST", regexp.MustCompile(`^/(repos/[^/]+/[^/]+|orgs/[^/]+|enterprises/[^/]+)/actions/runners/(registration-token|generate-jitconfig)$`), "a self-hosted runner"},
	{"POST PATCH", regexp.MustCompile(`^/(repos/[^/]+/[^/]+|orgs/[^/]+)/hooks(/[^/]+(/config)?)?$`), "a webhook"},
}

// mintsCredential: what the request would make, or "".
func mintsCredential(host, method, p string) string {
	if host != "api.github.com" {
		return ""
	}
	p = strings.ToLower(path.Clean("/" + p))
	for _, m := range githubMinting {
		if strings.Contains(" "+m.methods+" ", " "+strings.ToUpper(method)+" ") && m.path.MatchString(p) {
			return m.what
		}
	}
	return ""
}

// forwarder: a streaming reverse proxy to scheme://target, dialing through the floor, putting
// secrets in (HTTPS only), and logging each request. The bindings are read for every request, so
// `devvm secret unuse` applies to connections already open.
func forwarder(scheme, target, host string, port int) http.Handler {
	tlsConf := &tls.Config{RootCAs: upstreamCAs, ServerName: host}
	return &httputil.ReverseProxy{
		Transport: &http.Transport{
			DialContext:     func(ctx context.Context, _, _ string) (net.Conn, error) { return dial(ctx, host, port) },
			TLSClientConfig: tlsConf, ForceAttemptHTTP2: true,
			IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
		},
		FlushInterval: -1, // stream (SSE) instead of buffering
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = scheme, target
			// The host that was checked (and its secrets bound to), in its canonical form: not the
			// VM's Host header, which may differ in port or a trailing dot.
			pr.Out.Host = host
			if (scheme != "https" || port != 443) && (scheme != "http" || port != 80) {
				pr.Out.Host = net.JoinHostPort(host, fmt.Sprint(port))
			}
			pr.Out.Header.Del("Proxy-Authorization")
			pr.Out.Header.Del("Proxy-Connection")
			e := Event{Event: "forward", Host: host, Port: port, Method: pr.In.Method, Path: pr.In.URL.Path, Agent: pr.In.UserAgent()}
			if scheme == "https" && interceptPorts[port] { // secrets only ever go over TLS, to :443
				if bs := bindingsFor(host); len(bs) > 0 {
					e.Secrets = inject(pr.Out.Header, bs)
				}
			}
			record(e)
		},
		// The error stays in the proxy's own log: it can name addresses the VM shouldn't learn.
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			logf("%s:%d: %v", host, port, err)
			http.Error(w, "devvm proxy: couldn't reach "+host, http.StatusBadGateway)
		},
	}
}

// One already-accepted connection as a listener, for http.Server.Serve.
type oneConnListener struct {
	c    net.Conn
	once sync.Once
	done chan struct{}
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.c })
	if c != nil {
		return c, nil
	}
	<-l.done
	return nil, net.ErrClosed
}
func (l *oneConnListener) Close() error   { return nil }
func (l *oneConnListener) Addr() net.Addr { return l.c.LocalAddr() }

type closeNotifyConn struct {
	net.Conn
	once sync.Once
	done chan struct{}
}

func (c *closeNotifyConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return c.Conn.Close()
}
