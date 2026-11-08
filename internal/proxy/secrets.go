package proxy

// Secrets: the VM only ever holds placeholders. For hosts with a bound secret, the proxy terminates
// TLS with a certificate from the VM's own CA (trusted only inside that VM), replaces the placeholder in
// the binding's header with the real value, and forwards over a fresh TLS connection. Values come
// from devvm's own lookup (IO.Secret: the Keychain, devvm's secrets folder or an agent's login
// file), in this process, on demand, and are cached briefly; they're never logged or written
// anywhere.

import (
	"container/list"
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
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The kinds of Binding: where its value comes from.
const (
	// KindSecret is a secret devvm keeps (devvm secret set): in the Keychain, or in devvm's
	// secrets folder.
	KindSecret = "secret"
	// KindLogin is one of an agent's logins on the host: read from Agent's login file each time.
	KindLogin = "login"
)

// Binding is one of the VM's secret bindings: the hosts whose :443 requests get a value, the paths
// there, the header it goes in, the placeholder it replaces there, and where the value comes from
// (Kind, with a login's Agent). It holds no value. internal/secrets and internal/agentlogin make
// them, and devvm writes a VM's in a BindingList (Config.Secrets).
//
// A secret's placeholder is replaced wherever its header holds it, also inside Basic credentials
// (git sends x-access-token:TOKEN that way). A login's is replaced only when the header's one value
// is the placeholder alone, or for Authorization, Bearer and one space before it (the scheme in
// any case), so a VM can't put bytes of its own around the value: a provider that quotes part of
// a key it refuses could show the VM where the value starts.
type Binding struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // KindSecret or KindLogin
	// Agent is a login's agent, whose login file holds its value (codex, opencode, claude).
	Agent string   `json:"agent,omitempty"`
	Hosts []string `json:"hosts"` // host patterns, as in policy files
	// Path, when set, is the only path below which the value goes in: it starts and ends with /,
	// and a request's path must start with it, in its clean form (pathWithin).
	Path        string `json:"path,omitempty"`
	Header      string `json:"header"` // e.g. Authorization
	Placeholder string `json:"placeholder"`
}

// BindingList is the VM's bindings file (Config.Secrets), which devvm writes and the proxy reads
// again when it changes: names, kinds, hosts, headers and placeholders, never values.
type BindingList struct {
	Version  int       `json:"version"` // BindingListVersion
	Bindings []Binding `json:"bindings"`
}

// BindingListVersion is the bindings file's version: a file of another isn't read.
const BindingListVersion = 1

// bindingListMax bounds the bindings file the proxy reads.
const bindingListMax = 1 << 20

// Encode is the list as devvm writes it, of BindingListVersion, a line at its end.
func (l BindingList) Encode() []byte {
	l.Version = BindingListVersion
	if l.Bindings == nil {
		l.Bindings = []Binding{}
	}
	b, _ := json.Marshal(l) // only strings: it never fails
	return append(b, '\n')
}

// ReadBindings reads the bindings file at path: a regular file, not a link, that only its owner,
// this process's user, may read or write, at most bindingListMax long (readPrivate), of
// BindingListVersion, each binding of a kind the proxy knows.
func ReadBindings(path string) (BindingList, error) {
	b, err := readBindingsFile(path)
	if err != nil {
		return BindingList{}, err
	}
	return parseBindings(path, b)
}

func readBindingsFile(path string) ([]byte, error) { return readPrivate(path, bindingListMax, 0o077) }

// parseBindings is ReadBindings for the bytes b, read from path.
func parseBindings(path string, b []byte) (BindingList, error) {
	var l BindingList
	if err := json.Unmarshal(b, &l); err != nil {
		return BindingList{}, fmt.Errorf("%s: %w", path, err)
	}
	if l.Version != BindingListVersion {
		return BindingList{}, fmt.Errorf("%s: version %d", path, l.Version)
	}
	for _, x := range l.Bindings {
		if x.Kind != KindSecret && x.Kind != KindLogin {
			return BindingList{}, fmt.Errorf("%s: binding %q of kind %q", path, x.Name, x.Kind)
		}
		if x.Path != "" && !pathPrefixOK(x.Path) {
			return BindingList{}, fmt.Errorf("%s: binding %q's path %q", path, x.Name, x.Path)
		}
	}
	return l, nil
}

// pathPrefixOK reports whether p can be a binding's Path: it starts and ends with /, and is
// clean (no . or .. segment, no doubled slash), with nothing pathWithin refuses in it.
func pathPrefixOK(p string) bool {
	if len(p) < 2 || p[0] != '/' || p[len(p)-1] != '/' || strings.ContainsAny(p, `\;% `) {
		return false
	}
	return path.Clean(p)+"/" == p
}

// InterceptLines returns what a VM's allowlist gets for its bindings' hosts, as policy lines: each
// on 443 alone, since a binding is only intercepted there, each once, in order.
func InterceptLines(hosts []string) []string {
	var lines []string
	seen := map[string]bool{}
	for _, h := range hosts {
		if l := h + ":443"; !seen[l] {
			seen[l] = true
			lines = append(lines, l)
		}
	}
	return lines
}

// Lookup returns a binding's value: IO.Secret, which devvm sets to its secrets' store.
type Lookup func(ctx context.Context, b Binding) (string, error)

// secrets is a proxy's bindings (Config.Secrets) and the values it has looked up for them.
type secrets struct {
	// file is Config.Secrets, the bindings file (no values), re-read on change; nil: none. Its
	// lock is taken before valueMu, never after.
	file   *watched[[]Binding]
	lookup Lookup // IO.Secret: each binding's value
	// route is the company proxy each lookup's own requests go through (WithRoute), the one the
	// VM's requests go through: set as the proxy starts. nil: net/http's own.
	route func(*http.Request) (*url.URL, error)
	// roots is what each lookup's own TLS connections trust (WithRoots), the proxy's own: the
	// system roots and the user's CAs. nil: the system roots.
	roots *x509.CertPool
	log   *log.Logger
	// valueMu holds the values, by binding name, and is held through a lookup, so lookups go one
	// at a time.
	valueMu sync.Mutex
	values  map[string]cached
}

func newSecrets(path string, lookup Lookup, l *log.Logger) *secrets {
	s := &secrets{lookup: lookup, log: l, values: map[string]cached{}}
	if path != "" {
		s.file = &watched[[]Binding]{
			path: path, read: readBindingsFile,
			parse: func(b []byte) ([]Binding, error) {
				bl, err := parseBindings(path, b)
				if err == nil {
					l.Printf("secrets %s: %d bindings", path, len(bl.Bindings))
				}
				return bl.Bindings, err
			},
			onFail: func(err error) { l.Printf("secrets: %v", err) },
			// devvm rewrites the file when a value is set or removed: look values up afresh.
			changed: func() {
				s.valueMu.Lock()
				s.values = map[string]cached{}
				s.valueMu.Unlock()
			},
		}
	}
	return s
}

// all returns the bindings as the file has them now. A file that's gone, or that can't be read as
// ReadBindings reads it, has none: no value goes in until devvm writes it again.
func (s *secrets) all() []Binding {
	if s.file == nil {
		return nil
	}
	return s.file.get()
}

// forHost: the bindings that apply to host.
func (s *secrets) forHost(host string) []Binding {
	var out []Binding
	for _, b := range s.all() {
		for _, h := range b.Hosts {
			if r, ok := parseRule(h); ok && !r.deny && r.matchHost(strings.ToLower(host)) {
				out = append(out, b)
				break
			}
		}
	}
	return out
}

// forRequest: the bindings that apply to a request to host for u, as the proxy forwards it:
// forHost's, but a binding with a Path only when u's path is within it (pathWithin).
func (s *secrets) forRequest(host string, u *url.URL) []Binding {
	return slices.DeleteFunc(s.forHost(host), func(b Binding) bool { return b.Path != "" && !pathWithin(u, b.Path) })
}

// pathWithin reports whether u's path, as the proxy forwards it, starts with prefix (a binding's
// Path) in a form every server reads alike: no opaque URL, nothing percent-encoded (an encoded
// slash or dot that a server decodes could take the request outside prefix after the check), no
// backslash or ; (which some servers read as a separator, or drop with what follows), and the
// path clean already (no . or .. segment, no doubled slash).
func pathWithin(u *url.URL, prefix string) bool {
	p := u.Path
	if u.Opaque != "" || strings.Contains(u.EscapedPath(), "%") || strings.ContainsAny(p, `\;`) {
		return false
	}
	clean := path.Clean(p)
	if strings.HasSuffix(p, "/") && clean != "/" {
		clean += "/"
	}
	return clean == p && strings.HasPrefix(p, prefix)
}

// --- values ---

type cached struct {
	value string
	at    time.Time
}

// A value is cached for valueTTL; a login's (KindLogin: Codex, opencode, Claude MCP) for loginTTL,
// since logging out on the host changes nothing devvm sees.
const (
	valueTTL = 5 * time.Minute
	loginTTL = 30 * time.Second
)

// lookupTimeout bounds one lookup: the Keychain, or a login's file and its refresh.
const lookupTimeout = 20 * time.Second

func ttlOf(b Binding) time.Duration {
	if b.Kind == KindLogin {
		return loginTTL
	}
	return valueTTL
}

// value returns b's value from the lookup, cached (see ttlOf). Lookups go one at a time, so two
// requests never refresh a login at once.
func (s *secrets) value(b Binding) (string, error) {
	s.valueMu.Lock()
	defer s.valueMu.Unlock()
	if c, ok := s.values[b.Name]; ok && time.Since(c.at) < ttlOf(b) {
		return c.value, nil
	}
	if s.lookup == nil {
		return "", errors.New("no secret lookup")
	}
	// The value is cached for every request: no one request's end cancels it.
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout) //nolint:forbidigo // shared by every request
	defer cancel()
	if s.route != nil {
		ctx = WithRoute(ctx, s.route)
	}
	if s.roots != nil {
		ctx = WithRoots(ctx, s.roots)
	}
	v, err := s.lookup(ctx, b)
	if err != nil {
		return "", fmt.Errorf("secret %s: %w", b.Name, err)
	}
	if v = strings.TrimSpace(v); v == "" {
		return "", fmt.Errorf("secret %s is empty", b.Name)
	}
	s.values[b.Name] = cached{v, time.Now()}
	return v, nil
}

// inject puts each binding's value in its header, where it takes it (takes): only the binding's
// header is touched. It returns the names of the secrets it put in.
func (s *secrets) inject(h http.Header, bs []Binding) (used []string) {
	for _, b := range bs {
		vs := h.Values(b.Header)
		if !b.takes(vs) {
			if b.Kind == KindLogin && slices.ContainsFunc(vs, func(v string) bool { return strings.Contains(v, b.Placeholder) }) {
				s.log.Printf("secret %s: not put in: its %s header holds more than its placeholder", b.Name, b.Header)
			}
			continue
		}
		val, err := s.value(b)
		if err != nil {
			s.log.Printf("%v", err)
			return used
		}
		out := make([]string, len(vs))
		for i, v := range vs {
			out[i] = b.swap(v, val)
		}
		h.Del(b.Header)
		for _, v := range out {
			h.Add(b.Header, v)
		}
		// A login can have a binding for each of its headers, all of one name.
		if !slices.Contains(used, b.Name) {
			used = append(used, b.Name)
		}
	}
	return used
}

// takes reports whether b's value goes in the values vs of its header: for a secret, when one of
// them holds the placeholder, also inside Basic credentials; for a login, when there's one value
// and it's the placeholder alone, or Bearer and the placeholder in Authorization (loginValue). A
// login's header that holds the placeholder any other way, or twice, which a server may join into
// one value, keeps it, and the proxy's log says so.
func (b Binding) takes(vs []string) bool {
	if b.Kind != KindLogin {
		return slices.ContainsFunc(vs, func(v string) bool { return strings.Contains(v, b.Placeholder) || basicHas(v, b.Placeholder) })
	}
	return len(vs) == 1 && b.loginValue(vs[0])
}

// loginValue reports whether v is a login's header value the proxy fills in: the placeholder
// alone, or in Authorization, Bearer (in any case), one space and the placeholder.
func (b Binding) loginValue(v string) bool {
	if v == b.Placeholder {
		return true
	}
	scheme, rest, ok := strings.Cut(v, " ")
	return ok && strings.EqualFold(b.Header, "Authorization") && strings.EqualFold(scheme, "Bearer") && rest == b.Placeholder
}

// swap is v, a value of b's header that takes b's value, with val in place of the placeholder: a
// login's is the whole value, or what follows Bearer; a secret's is every copy of it, also inside
// Basic credentials.
func (b Binding) swap(v, val string) string {
	if b.Kind == KindLogin {
		if !b.loginValue(v) {
			return v
		}
		return strings.TrimSuffix(v, b.Placeholder) + val
	}
	nv := strings.ReplaceAll(v, b.Placeholder, val)
	if raw, ok := basic(nv); ok && strings.Contains(raw, b.Placeholder) {
		nv = "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(raw, b.Placeholder, val)))
	}
	return nv
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

// ca is the VM's CA as its proxy has it (Config.CA): the certificate and key it signs leaves with,
// the one key every leaf has, the leaves it has minted, by host, under mu, and its next CA
// (Config.CANext), nil for none. It keeps at most leafMax leaves, the most recently used at the
// front of order, and the one used least recently goes when another would pass it.
type ca struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	leafKey *ecdsa.PrivateKey
	mu      sync.Mutex
	leaves  map[string]*list.Element // each a *leaf in order
	order   *list.List
	leafMax int
	next    *ca
}

// leaf is a certificate the CA minted, and its host.
type leaf struct {
	host string
	cert *tls.Certificate
}

// leafCacheMax is how many leaves a CA keeps: a binding for one host needs one, and one with a
// wildcard host (*.example.com) one for each name the VM asks for, which could grow without end.
// Minting one again costs one signature, and each leaf kept takes a few KiB, so 1024 keep every
// host an agent's session uses in a few MiB.
const leafCacheMax = 1024

// Each VM has CAs of its own (trust.InitCA), which no other VM trusts: the current one and the next
// one, which every launch installs in the VM beside it. The next CA starts before the current one
// ends, and the proxy signs with it from successorSkew after its start (signer), the moment from
// which the VM's next launch makes it the current one, so a VM that runs on without a launch goes
// on verifying the proxy's certificates, and the proxy needs no restart. A proxy keeps the CAs it
// started with, as its VM trusts no other, so one whose VM runs past the next CA's end can
// intercept nothing: leafFor mints no leaf past the end, and caEndedRefusal refuses the CONNECT
// before its tunnel opens, with caEndedReason, which says to open the VM again.

// successorSkew is how long after its start the next CA signs: trust.InitCA dates it that early, so
// a VM whose clock is behind by up to that still sees it started.
const successorSkew = time.Hour

// signer is the CA that signs at now: the next one from successorSkew after its start, else c.
func (c *ca) signer(now time.Time) *ca {
	if n := c.next; n != nil && !now.Before(n.cert.NotBefore.Add(successorSkew)) {
		return n
	}
	return c
}

// writeAtomic replaces path with data, mode mode, whole or not at all: a
// temporary file in its folder (.<name>.tmp-*), renamed over it. It's the
// proxy's one writer of whole files (the proxy imports only the standard
// library, so not devvm's state.WriteFile): the Nix cache's files and the
// rotated log.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
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
	return os.Rename(tmp.Name(), path) //nolint:gosec // devvm's own files, or a Nix cache file nixPath named
}

// caFileMax bounds a CA's certificate and key files.
const caFileMax = 64 << 10

// loadCA reads a CA's certificate and key files (Config.CA and Config.CAKey, or Config.CANext and
// Config.CANextKey), neither a symbolic link: the certificate only its owner may write, the key
// only its owner may read or write (readPrivate).
func loadCA(certPath, keyPath string) (*ca, error) {
	certPEM, err := readPrivate(certPath, caFileMax, 0o022)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readPrivate(keyPath, caFileMax, 0o077)
	if err != nil {
		return nil, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	k, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("the CA key isn't ECDSA")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	return newCA(cert, k)
}

// newCA is the CA cert, signing with key, with a leaf key of its own and no leaves yet.
func newCA(cert *x509.Certificate, key *ecdsa.PrivateKey) (*ca, error) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &ca{cert: cert, key: key, leafKey: leafKey, leaves: map[string]*list.Element{}, order: list.New(), leafMax: leafCacheMax}, nil
}

// upstreamPool is what the proxy's own TLS connections trust: the CA certificates in pem
// (Config.UpstreamCAs: the user's, checked by devvm) as well as the system roots.
func upstreamPool(pem []byte) (*x509.CertPool, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("upstream_cas holds no certificates")
	}
	return pool, nil
}

// pem is the CA's certificate and its next one's, for /ca.pem; nil for no CA.
func (c *ca) pem() []byte {
	if c == nil {
		return nil
	}
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.cert.Raw})
	if c.next != nil {
		b = append(b, c.next.pem()...)
	}
	return b
}

// errCAEnded: the CA is past its NotAfter, so a leaf it signed would verify nowhere.
var errCAEnded = errors.New("devvm's CA has ended")

// caEndedReason is the log's reason for a tunnel refused because the CA has ended.
const caEndedReason = "devvm's CA has ended: open the VM again (devvm in its folder) for a new one"

// ended reports whether the CA that signs at now (signer) is past its NotAfter.
func (c *ca) ended(now time.Time) bool { return !now.Before(c.signer(now).cert.NotAfter) }

// caEndedRefusal refuses a CONNECT to be intercepted (e) once the CA has ended, before the tunnel
// opens: 403, with the reason and no allow command, since no rule would help, and logged as a
// refused block. It reports whether it refused.
func (s *server) caEndedRefusal(w http.ResponseWriter, e Event) bool {
	if !s.ca.ended(time.Now()) {
		return false
	}
	e.Event, e.Reason, e.Refused = "blocked", caEndedReason, true
	s.events.record(e)
	w.Header().Set("X-Devvm-Blocked", e.Host)
	http.Error(w, "devvm proxy: "+net.JoinHostPort(e.Host, strconv.Itoa(e.Port))+" is refused: "+caEndedReason, http.StatusForbidden)
	return true
}

// leafFor mints (and caches) a certificate for host, signed by the CA that signs now (signer).
func (c *ca) leafFor(host string) (*tls.Certificate, error) {
	now := time.Now()
	return c.signer(now).mint(host, now)
}

// mint mints (and caches) a certificate for host, signed by c itself: for a day, and never past
// the CA's own NotAfter or before its NotBefore, since a client checks the whole chain. One that
// ends with the CA is kept until then. Once the CA has ended there's none (errCAEnded).
func (c *ca) mint(host string, now time.Time) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !now.Before(c.cert.NotAfter) {
		return nil, errCAEnded
	}
	if el, ok := c.leaves[host]; ok {
		l := el.Value.(*leaf).cert
		if now.Before(l.Leaf.NotAfter) && (l.Leaf.NotAfter.Sub(now) > time.Hour || l.Leaf.NotAfter.Equal(c.cert.NotAfter)) {
			c.order.MoveToFront(el)
			return l, nil
		}
	}
	notBefore, notAfter := now.Add(-time.Hour), now.Add(24*time.Hour)
	if notBefore.Before(c.cert.NotBefore) {
		notBefore = c.cert.NotBefore
	}
	if notAfter.After(c.cert.NotAfter) {
		notAfter = c.cert.NotAfter
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &c.leafKey.PublicKey, c.key)
	if err != nil {
		return nil, err
	}
	parsed, _ := x509.ParseCertificate(der)
	l := &tls.Certificate{Certificate: [][]byte{der, c.cert.Raw}, PrivateKey: c.leafKey, Leaf: parsed}
	c.keep(host, l)
	return l, nil
}

// keep caches l as host's leaf, the most recently used, and lets the least recently used go past
// leafMax; c.mu is held.
func (c *ca) keep(host string, l *tls.Certificate) {
	if el, ok := c.leaves[host]; ok {
		el.Value.(*leaf).cert = l
		c.order.MoveToFront(el)
		return
	}
	c.leaves[host] = c.order.PushFront(&leaf{host: host, cert: l})
	for c.order.Len() > c.leafMax {
		old := c.order.Back()
		delete(c.leaves, old.Value.(*leaf).host)
		c.order.Remove(old)
	}
}

// --- interception ---

// intercept serves one CONNECT tunnel to host:443 whose host has bindings: TLS with a minted
// certificate on the client side, HTTP/1.1 or HTTP/2 inside, and each request forwarded upstream
// over a new TLS connection with the secrets put in. Each request must be for host itself: the
// secrets are bound to it, so another Host (a site on the same CDN, say) gets 421. The handshake
// with the VM gets 30 seconds.
func (s *server) intercept(ctx context.Context, client net.Conn, host string, port int, e Event) {
	tconn := tls.Server(client, &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.ca.leafFor(host)
		},
	})
	hctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := tconn.HandshakeContext(hctx)
	cancel()
	if err != nil {
		e.Event, e.Reason = "blocked", "TLS with the VM failed (is the devvm CA installed?): "+err.Error()
		s.events.record(e)
		_ = client.Close()
		return
	}
	e.Event = "intercept"
	s.events.record(e)
	cn := &closeNotifyConn{Conn: tconn, done: make(chan struct{})}
	fwd, done := s.forwarder("https", net.JoinHostPort(host, fmt.Sprint(port)), host, port)
	defer done()
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if h, _ := splitHostPort(r.Host, port); h != host {
				http.Error(w, "devvm proxy: this connection is for "+host, http.StatusMisdirectedRequest)
				return
			}
			fwd.ServeHTTP(w, r)
		}),
		Protocols:         new(http.Protocols),
		IdleTimeout:       10 * time.Minute,
		ReadHeaderTimeout: 30 * time.Second,
		ErrorLog:          s.log,
	}
	// TLS is already done here, so the server sees plain HTTP/1.1, or HTTP/2 with prior knowledge.
	srv.Protocols.SetHTTP1(true)
	srv.Protocols.SetUnencryptedHTTP2(true)
	_ = srv.Serve(&oneConnListener{c: cn, done: cn.done})
}

// GitHub API calls that make a credential of their own with the token the proxy puts in: an SSH or
// GPG key on your account, a repo's deploy key, a self-hosted runner's registration, a webhook
// (which sends a repo's events anywhere). Each would keep working after devvm secret unuse. A repo
// is named by owner and name, or by its ID (/repositories/ID), an org by its name or its ID.
var githubMinting = []struct {
	methods string
	path    *regexp.Regexp
	what    string
}{
	{"POST", regexp.MustCompile(`^/user/(keys|ssh_signing_keys|gpg_keys)$`), "a key on your GitHub account"},
	{"POST", regexp.MustCompile(`^/(repos/[^/]+/[^/]+|repositories/[^/]+)/keys$`), "a deploy key"},
	{"POST", regexp.MustCompile(`^/(repos/[^/]+/[^/]+|repositories/[^/]+|orgs/[^/]+|organizations/[^/]+|enterprises/[^/]+)/actions/runners/(registration-token|generate-jitconfig)$`), "a self-hosted runner"},
	{"POST PATCH", regexp.MustCompile(`^/(repos/[^/]+/[^/]+|repositories/[^/]+|orgs/[^/]+|organizations/[^/]+)/hooks(/[^/]+(/config)?)?$`), "a webhook"},
}

// refuseMinting answers a request that would make a credential with the token the proxy puts in
// (mintsCredential) with 403, logged as a refused block, and reports whether it did. Every HTTPS
// request the proxy forwards asks it first, in a tunnel it intercepts or in absolute form.
func (s *server) refuseMinting(w http.ResponseWriter, r *http.Request, host string, port int) bool {
	what := mintsCredential(host, r.Method, r.URL.Path)
	if what == "" {
		return false
	}
	s.events.record(Event{
		Event: "blocked", Host: host, Port: port, Method: r.Method, Path: r.URL.Path, Refused: true,
		Reason: "makes " + what + " with your token, which would outlast the VM; do it on the host",
	})
	http.Error(w, "devvm proxy: refused: this makes "+what+" with your token (do it on the host)", http.StatusForbidden)
	return true
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

// forwarder: a streaming reverse proxy to scheme://target, dialing through the floor (or through
// the upstream), putting secrets in (HTTPS only, which refuses a request that would make a
// credential with them first: refuseMinting), and logging each request. The bindings are read
// for every request, so `devvm secret unuse` applies to connections already open. HTTPS goes over
// TLS with the system's roots and the user's CAs, through an upstream's CONNECT tunnel when one
// applies; plain HTTP through an upstream is sent to it in absolute form, with its credentials.
// done closes the connections it keeps open for further requests, once its caller has no more.
func (s *server) forwarder(scheme, target, host string, port int) (fwd http.Handler, done func()) {
	tlsConf := &tls.Config{RootCAs: s.upstreamCAs, ServerName: host}
	tr := &http.Transport{
		DialContext:     func(ctx context.Context, _, _ string) (net.Conn, error) { return s.egress.dialOut(ctx, host, port) },
		TLSClientConfig: tlsConf, ForceAttemptHTTP2: true,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second,
	}
	up := s.egress.up.route(host, port, scheme == "http")
	if scheme == "http" {
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) { return s.egress.dial(ctx, host, port) }
		if up != nil {
			// The upstream connects to what the request line names: the host that was checked,
			// in its canonical form, never the VM's spelling of it.
			if target = net.JoinHostPort(host, fmt.Sprint(port)); port == 80 {
				target = host
				if strings.Contains(host, ":") {
					target = "[" + host + "]"
				}
			}
			// No credentials in the URL: they go in the request's header (Rewrite), so no error
			// the transport makes can hold them.
			u := &url.URL{Scheme: "http", Host: up.addr}
			tr.Proxy = func(*http.Request) (*url.URL, error) { return u, nil }
			tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) { return up.dialUpstream(ctx) }
		}
	}
	rp := &httputil.ReverseProxy{
		Transport:     tr,
		ErrorLog:      s.log,
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
			if up != nil {
				e.Upstream = up.addr
				// Only plain HTTP carries them, to the upstream itself; HTTPS's go in the CONNECT.
				if scheme == "http" && up.auth != "" {
					pr.Out.Header.Set("Proxy-Authorization", up.auth)
				}
			}
			// A token endpoint's host's replies come as they are, or as gzip that Go's transport
			// undoes, so the proxy reads them (capture.go).
			if s.captures.has(host) {
				pr.Out.Header.Del("Accept-Encoding")
			}
			// A captured request goes to the endpoint's own path, never the VM's spelling of it,
			// which another handler there could read apart.
			if c := captured(pr.In.Context()); c != nil {
				pr.Out.URL.Path, pr.Out.URL.RawPath, pr.Out.URL.RawQuery = c.path, "", ""
			}
			// Secrets only ever go over TLS, to :443, each for the path that goes out.
			if scheme == "https" && s.interceptPorts[port] {
				if bs := s.secrets.forRequest(host, pr.Out.URL); len(bs) > 0 {
					e.Secrets = s.secrets.inject(pr.Out.Header, bs)
				}
			}
			s.events.record(e)
		},
		ModifyResponse: s.captureResponse(host, port), //nolint:bodyclose // ReverseProxy closes the reply's body; captureResponse closes the ones it replaces

		// The error stays in the proxy's own log: it can name addresses the VM shouldn't learn.
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, errCaptureRefused) {
				s.refuseCapture(w, r, host, port, err)
				return
			}
			s.log.Printf("%s:%d: %v", host, port, err)
			http.Error(w, "devvm proxy: couldn't reach "+host, http.StatusBadGateway)
		},
	}
	if scheme != "https" {
		return rp, tr.CloseIdleConnections
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.refuseMinting(w, r, host, port) {
			return
		}
		// An agent's token endpoint's host: no informational reply reaches the VM unread, and a
		// login at the endpoint is captured (capture.go).
		if s.captures.has(host) {
			w = no1xx{w}
		}
		if agents, at := s.captures.agents(host, r); len(agents) > 0 && s.interceptPorts[port] {
			var ok bool
			if r, ok = s.captureRequest(w, r, host, port, agents, at); !ok {
				return
			}
		}
		rp.ServeHTTP(w, r)
	}), tr.CloseIdleConnections
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
