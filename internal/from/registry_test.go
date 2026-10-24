package from

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestParseRef(t *testing.T) {
	for _, c := range []struct{ in, host, repo, tag string }{
		{"node:24", DockerHub, "library/node", "24"},
		{"alpine", DockerHub, "library/alpine", "latest"},
		{"docker.io/library/debian:13", DockerHub, "library/debian", "13"},
		{"index.docker.io/library/debian", DockerHub, "library/debian", "latest"},
		{"bitnami/redis:7", DockerHub, "bitnami/redis", "7"},
		{"ghcr.io/astral-sh/uv:0.9", "ghcr.io", "astral-sh/uv", "0.9"},
		{"quay.io/org/sub/img", "quay.io", "org/sub/img", "latest"},
		{"localhost:5000/app:dev", "localhost:5000", "app", "dev"},
		{"localhost/app", "localhost", "app", "latest"},
		{"node:24@sha256:" + strings.Repeat("a", 64), DockerHub, "library/node", "24"},
		{"ghcr.io/o/r@sha256:" + strings.Repeat("a", 64), "ghcr.io", "o/r", "latest"},
	} {
		if got := ParseRef(c.in); got != (Ref{c.host, c.repo, c.tag}) {
			t.Errorf("ParseRef(%q) = %+v, want %s %s %s", c.in, got, c.host, c.repo, c.tag)
		}
	}
}

// FuzzParseRef: any reference parses, into a tag without / or :, and a
// valid one parses the same once written out in full.
func FuzzParseRef(f *testing.F) {
	for _, s := range []string{"node:24", "alpine", "ghcr.io/o/r:1", "localhost:5000/app:dev", "docker.io/x", "a@b", "node:", "/x", ":"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		r := ParseRef(s)
		if r.Host == "" || strings.ContainsAny(r.Tag, "/:") || strings.Contains(r.Repo, "@") {
			t.Fatalf("ParseRef(%q) = %+v", s, r)
		}
		if !Valid(s) {
			return
		}
		if again := ParseRef(r.Host + "/" + r.Repo + ":" + r.Tag); again != r {
			t.Fatalf("ParseRef(%q) = %+v, but its full form parses as %+v", s, r, again)
		}
	})
}

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"node:24":              "node",
		"node":                 "node",
		"ghcr.io/o/r:1":        "ghcr.io/o/r",
		"localhost:5000/app":   "localhost:5000/app",
		"localhost:5000/a:dev": "localhost:5000/a",
		"node:24@sha256:aa":    "node",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeRegistry is a registry (and its token service) over TLS: every
// manifest needs a token first. Manifests maps repo:tag to a digest.
type fakeRegistry struct {
	srv       *httptest.Server
	mu        sync.Mutex
	manifests map[string]string
	realm     string // the token service's address; "" for its own /token
	calls     []string
}

func newFakeRegistry(t *testing.T) *fakeRegistry {
	t.Helper()
	f := &fakeRegistry{manifests: map[string]string{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeRegistry) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := r.Method + " " + r.URL.Path
	if r.URL.Path == "/token" {
		call += "?" + r.URL.RawQuery
	}
	if a := r.Header.Get("Authorization"); a != "" {
		call += " (" + a + ")"
	}
	f.calls = append(f.calls, call)
	if r.URL.Path == "/token" {
		_, _ = w.Write([]byte(`{"token":"stub-token","expires_in":300}`))
		return
	}
	repoTag, ok := strings.CutPrefix(r.URL.Path, "/v2/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	repoTag = strings.Replace(repoTag, "/manifests/", ":", 1)
	if r.Header.Get("Authorization") != "Bearer stub-token" {
		realm := f.realm
		if realm == "" {
			realm = f.srv.URL + "/token"
		}
		repo, _, _ := strings.Cut(repoTag, ":")
		w.Header().Set("Www-Authenticate", `Bearer realm="`+realm+`",service="registry.test",scope="repository:`+repo+`:pull"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json") {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}
	d, ok := f.manifests[repoTag]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Docker-Content-Digest", d)
}

func (f *fakeRegistry) registry() *Registry {
	return &Registry{Client: f.srv.Client(), Base: func(string) string { return f.srv.URL }}
}

func (f *fakeRegistry) set(repoTag, digest string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manifests[repoTag] = digest
}

func digestOf(c string) string { return "sha256:" + strings.Repeat(c, 64) }

func TestDigestTokenThenManifest(t *testing.T) {
	f := newFakeRegistry(t)
	f.set("library/node:24", digestOf("a"))
	d, err := f.registry().Digest(t.Context(), "node:24")
	if err != nil || d != digestOf("a") {
		t.Fatalf("Digest = %q, %v", d, err)
	}
	want := []string{
		"HEAD /v2/library/node/manifests/24",
		"GET /token?scope=repository%3Alibrary%2Fnode%3Apull&service=registry.test",
		"HEAD /v2/library/node/manifests/24 (Bearer stub-token)",
	}
	if strings.Join(f.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(f.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestDigestNotFoundAndUnreachable(t *testing.T) {
	retryDelay = 0
	f := newFakeRegistry(t)
	if _, err := f.registry().Digest(t.Context(), "ghcr.io/someone/private:1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing image: %v", err)
	}
	// A digest that isn't one is no digest.
	f.set("library/node:24", "sha256:short")
	if _, err := f.registry().Digest(t.Context(), "node:24"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a bad digest: %v", err)
	}
	// Nothing listening: unreachable, after retries.
	down := httptest.NewTLSServer(http.NotFoundHandler())
	r := &Registry{Client: down.Client(), Base: func(string) string { return down.URL }}
	down.Close()
	if _, err := r.Digest(t.Context(), "node:24"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("no registry: %v", err)
	}
	// One that keeps failing is unreachable too.
	failing := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failing.Close()
	r = &Registry{Client: failing.Client(), Base: func(string) string { return failing.URL }}
	if _, err := r.Digest(t.Context(), "node:24"); !errors.Is(err, ErrUnreachable) {
		t.Errorf("a failing registry: %v", err)
	}
	// A cancelled lookup says so.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.registry().Digest(ctx, "node:24"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
}

// A registry can't point devvm's token request at a host file or a plain
// http address: the realm must be https.
func TestDigestRealmMustBeHTTPS(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(secret, []byte(`{"access_token":"HOST-SECRET"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"stub-token"}`))
	}))
	defer plain.Close()
	f := newFakeRegistry(t)
	f.set("library/node:24", digestOf("a"))
	for _, realm := range []string{(&url.URL{Scheme: "file", Path: secret}).String(), plain.URL + "/token", "/token"} {
		f.realm = realm
		f.calls = nil
		if _, err := f.registry().Digest(t.Context(), "node:24"); !errors.Is(err, ErrNotFound) {
			t.Errorf("realm %s: %v", realm, err)
		}
		if strings.Contains(strings.Join(f.calls, "\n"), "Bearer") {
			t.Errorf("realm %s: a token was used: %q", realm, f.calls)
		}
	}
}
