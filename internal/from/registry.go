package from

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// DockerHub is the registry of references that name none.
const DockerHub = "registry-1.docker.io"

// Ref is an image reference as Docker reads it.
type Ref struct {
	Host string // the registry: registry-1.docker.io, ghcr.io, localhost:5000
	Repo string // library/node, astral-sh/uv
	Tag  string // latest when the reference names none
}

// ParseRef splits a reference the way Docker does: node:24 is
// registry-1.docker.io library/node 24; the first part is a registry when
// it has a dot or a port, or is localhost (ghcr.io/o/r:t,
// localhost:5000/r). A digest (@sha256:…) is left out.
func ParseRef(s string) Ref {
	r, _, _ := strings.Cut(s, "@")
	ref := Ref{Host: DockerHub, Tag: "latest"}
	if first, rest, ok := strings.Cut(r, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		ref.Host, r = first, rest
	}
	if ref.Host == "docker.io" || ref.Host == "index.docker.io" {
		ref.Host = DockerHub
	}
	last := r[strings.LastIndex(r, "/")+1:]
	if i := strings.LastIndex(last, ":"); i >= 0 {
		ref.Tag = last[i+1:]
		r = r[:len(r)-len(last)+i]
	}
	if ref.Host == DockerHub && !strings.Contains(r, "/") {
		r = "library/" + r
	}
	ref.Repo = r
	return ref
}

// ErrUnreachable is a registry that can't be reached (offline), or that
// keeps failing.
var ErrUnreachable = errors.New("the registry can't be reached")

// ErrNotFound is an image the registry doesn't have, or won't give devvm
// (a private one).
var ErrNotFound = errors.New("the registry has no such image")

const (
	timeout  = 30 * time.Second // per request, body included
	maxToken = 1 << 20          // a token response is a few KB
	retries  = 2
)

// The manifest kinds asked for: an index (every architecture's) first.
const accept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, " +
	"application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Registry asks registries for image digests, anonymously.
type Registry struct {
	Client *http.Client
	// Base is the address of the registry at host: https://host.
	Base func(host string) string
	// RetryDelay is the wait before the first retry; it doubles after each.
	RetryDelay time.Duration
}

// NewRegistry returns a Registry on the internet.
func NewRegistry() *Registry {
	return &Registry{Client: NewClient(), Base: func(host string) string { return "https://" + host }, RetryDelay: time.Second}
}

// NewClient returns the HTTP client a Registry uses: a timeout per
// request, and redirects to https only.
func NewClient() *http.Client {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	return &http.Client{Transport: t.Clone(), Timeout: timeout, CheckRedirect: httpsOnly}
}

// NewPublicRegistry returns a Registry that reaches only the internet's
// addresses, for an image a project's file names: never this machine's
// (loopback), its network's (private, link-local, shared) or another
// non-public one, whatever a name resolves to. A proxy the environment
// names is reached as it is.
func NewPublicRegistry() *Registry {
	r := NewRegistry()
	t, ok := r.Client.Transport.(*http.Transport)
	if !ok {
		return r
	}
	var proxies sync.Map
	t.Proxy = func(req *http.Request) (*url.URL, error) {
		u, err := http.ProxyFromEnvironment(req)
		if u != nil {
			proxies.Store(proxyAddr(u), true)
		}
		return u, err
	}
	direct := &net.Dialer{Timeout: timeout, KeepAlive: timeout}
	guarded := &net.Dialer{Timeout: timeout, KeepAlive: timeout, Control: publicOnly}
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if _, ok := proxies.Load(addr); ok {
			return direct.DialContext(ctx, network, addr)
		}
		return guarded.DialContext(ctx, network, addr)
	}
	return r
}

// proxyAddr is the address a Transport dials for a proxy: its host and
// port, the scheme's default port when it names none.
func proxyAddr(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[u.Scheme]
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// publicOnly refuses a connection to an address that isn't the
// internet's: ErrNotFound, as the image isn't public.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !publicAddr(ip) {
		return fmt.Errorf("%w: %s isn't on the internet", ErrNotFound, host)
	}
	return nil
}

// nonPublic returns the IPv4 blocks besides Go's own classes that no
// registry anyone reaches is in: "this network", shared address space
// (carrier-grade NAT, smolvm's gateway), IETF protocol assignments,
// benchmarking, and reserved.
func nonPublic() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("240.0.0.0/4"),
	}
}

// publicAddr reports whether ip is an internet address: not loopback,
// private, link-local, multicast, unspecified or otherwise reserved.
func publicAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, p := range nonPublic() {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// errRedirect is a redirect refused: trying again won't help.
var errRedirect = errors.New("redirect refused")

func httpsOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("%w: too many", errRedirect)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("%w: to %s", errRedirect, req.URL.Scheme)
	}
	return nil
}

// Digest returns the digest (sha256:…) of the image a reference names,
// from a HEAD request for its manifest, which doesn't count against
// Docker Hub's pull limit. A registry that asks for a token gets an
// anonymous one, from its token service: an https address only, so a
// registry can't point devvm at a file or another scheme. A pinned
// reference's manifest is asked for by its digest. It fails with
// ErrUnreachable or ErrNotFound.
func (r *Registry) Digest(ctx context.Context, ref string) (string, error) {
	p := ParseRef(ref)
	tag := p.Tag
	if _, d, ok := strings.Cut(ref, "@"); ok {
		tag = d
	}
	u := r.Base(p.Host) + "/v2/" + p.Repo + "/manifests/" + tag
	h, err := r.head(ctx, u, "")
	if err != nil {
		return "", err
	}
	if h.status == http.StatusUnauthorized {
		token, err := r.token(ctx, p, h.challenge)
		if err != nil {
			return "", err
		}
		if h, err = r.head(ctx, u, token); err != nil {
			return "", err
		}
	}
	if h.status != http.StatusOK || !digestPattern.MatchString(h.digest) {
		return "", fmt.Errorf("%w: %s answered %d", ErrNotFound, p.Host, h.status)
	}
	return h.digest, nil
}

// answer is what a manifest HEAD request got.
type answer struct {
	status    int
	challenge string // the last WWW-Authenticate
	digest    string // Docker-Content-Digest
}

func (r *Registry) head(ctx context.Context, u, token string) (answer, error) {
	var a answer
	err := r.retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, http.NoBody)
		if err != nil {
			return false, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		req.Header.Set("Accept", accept)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := r.Client.Do(req)
		if err != nil {
			return !errors.Is(err, errRedirect), err
		}
		_ = resp.Body.Close()
		if transient(resp.StatusCode) {
			return true, fmt.Errorf("%s: %s", u, resp.Status)
		}
		a = answer{status: resp.StatusCode, digest: resp.Header.Get("Docker-Content-Digest")}
		if c := resp.Header.Values("Www-Authenticate"); len(c) > 0 {
			a.challenge = c[len(c)-1]
		}
		return false, nil
	})
	return a, err
}

var challengeParam = regexp.MustCompile(`([A-Za-z]+)="([^"]*)"`)

// token returns an anonymous pull token for the repository, from the
// service a Bearer challenge names.
func (r *Registry) token(ctx context.Context, p Ref, challenge string) (string, error) {
	params := map[string]string{}
	for _, m := range challengeParam.FindAllStringSubmatch(challenge, -1) {
		params[strings.ToLower(m[1])] = m[2]
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm.Scheme != "https" || realm.Host == "" || !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("%w: no https token service", ErrNotFound)
	}
	q := realm.Query()
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	scope := params["scope"]
	if scope == "" {
		scope = "repository:" + p.Repo + ":pull"
	}
	q.Set("scope", scope)
	realm.RawQuery = q.Encode()

	var token string
	err = r.retry(ctx, func() (bool, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), http.NoBody)
		if err != nil {
			return false, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		resp, err := r.Client.Do(req)
		if err != nil {
			return !errors.Is(err, errRedirect), err
		}
		defer func() { _ = resp.Body.Close() }()
		if transient(resp.StatusCode) {
			return true, fmt.Errorf("%s: %s", realm.Host, resp.Status)
		}
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("%w: no token: %s", ErrNotFound, resp.Status)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxToken+1))
		if err != nil {
			return true, err
		}
		var t struct {
			Token       string `json:"token"`
			AccessToken string `json:"access_token"`
		}
		if len(body) > maxToken || json.Unmarshal(body, &t) != nil {
			return false, fmt.Errorf("%w: no token", ErrNotFound)
		}
		token = t.Token
		if token == "" {
			token = t.AccessToken
		}
		if !tokenPattern.MatchString(token) {
			return false, fmt.Errorf("%w: no token", ErrNotFound)
		}
		return false, nil
	})
	return token, err
}

// tokenPattern is what a token can be in a header: printable, no spaces.
var tokenPattern = regexp.MustCompile(`^[!-~]+$`)

func transient(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
}

// retry runs try until it succeeds, fails for good, or has been retried
// retries times. A failure that never got an answer, or kept getting a
// server error, is ErrUnreachable.
func (r *Registry) retry(ctx context.Context, try func() (again bool, err error)) error {
	delay := r.RetryDelay
	for n := 0; ; n++ {
		again, err := try()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrNotFound) {
			return err
		}
		if !again {
			return fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		if n == retries {
			return fmt.Errorf("%w: %w", ErrUnreachable, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
