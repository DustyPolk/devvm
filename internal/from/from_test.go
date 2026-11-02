package from

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

func TestValidAndCheck(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"node:24", "ghcr.io/o/i@sha256:0123", "localhost:5000/a_b/c-d:1.2+x"} {
		if !Valid(s) || Check(s) != nil {
			t.Errorf("%q refused", s)
		}
	}
	for _, s := range []string{"node:24\nimage=x", "-x", " node", "node 24", "a;b", "$(x)", "a\x1b[2J"} {
		if Valid(s) {
			t.Errorf("%q accepted", s)
		}
		if msg.IDOf(Check(s)) != "bad_from" {
			t.Errorf("Check(%q) = %v", s, Check(s))
		}
	}
	if Check("") != nil {
		t.Error(`Check("") refused`)
	}
}

// testLookup is a Lookup on a fake registry, with a Docker that has the
// images in docker (ref to ID) and counts its calls, warnings collected.
type testLookup struct {
	*Lookup
	reg         *fakeRegistry
	docker      map[string]string
	dockerCalls int
	warned      msg.Recorder
}

func newTestLookup(t *testing.T) *testLookup {
	t.Helper()
	dir := t.TempDir()
	tl := &testLookup{reg: newFakeRegistry(t), docker: map[string]string{}}
	tl.Lookup = &Lookup{
		Dirs:     paths.Dirs{Data: filepath.Join(dir, "data"), Config: filepath.Join(dir, "config")},
		Registry: tl.reg.registry(),
		// The fake is on this machine's loopback, which
		// NewPublicRegistry's never reaches.
		PublicRegistry: tl.reg.registry(),
		DockerID: func(_ context.Context, ref string) (string, bool) {
			tl.dockerCalls++
			id, ok := tl.docker[ref]
			return id, ok
		},
		Warn: tl.warned.Add,
	}
	return tl
}

// offline makes the registry unreachable.
func (tl *testLookup) offline(t *testing.T) {
	t.Helper()
	down := httptest.NewTLSServer(http.NotFoundHandler())
	tl.Registry = &Registry{Client: down.Client(), Base: func(string) string { return down.URL }}
	tl.PublicRegistry = tl.Registry
	down.Close()
}

func (tl *testLookup) recorded(t *testing.T, ref string) string {
	t.Helper()
	d, err := Recorded(tl.Dirs, ref)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRefreshRecordsTheRegistrysDigestPerTag(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	tl.reg.set("library/node:24", digestOf("a"))
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "node:24"); d != "node@"+digestOf("a") {
		t.Errorf("recorded %q", d)
	}
	tl.reg.set("library/node:24", digestOf("b"))
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "node:24"); d != "node@"+digestOf("b") {
		t.Errorf("after the tag moved: %q", d)
	}
	tl.reg.set("o/r:1", digestOf("c"))
	if err := tl.Refresh(t.Context(), "ghcr.io/o/r:1"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "ghcr.io/o/r:1"); d != "ghcr.io/o/r@"+digestOf("c") {
		t.Errorf("ghcr.io: %q", d)
	}
	// Docker isn't asked about an image its registry has.
	if tl.dockerCalls != 0 || len(tl.warned.Names()) != 0 {
		t.Errorf("docker asked %d times, warnings %q", tl.dockerCalls, tl.warned.Names())
	}
}

func TestRefreshDockerOnlyImage(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	tl.docker["myapp:dev"] = digestOf("d")
	if err := tl.Refresh(t.Context(), "myapp:dev"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "myapp:dev"); d != digestOf("d") {
		t.Errorf("recorded %q", d)
	}
	// Docker has pulled images too: the registry says whether there's a
	// newer one.
	tl.docker["node:24"] = digestOf("e")
	tl.reg.set("library/node:24", digestOf("a"))
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "node:24"); d != "node@"+digestOf("a") {
		t.Errorf("a pulled image: %q", d)
	}
}

func TestRefreshOffline(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	tl.reg.set("library/node:24", digestOf("a"))
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	tl.offline(t)
	// Nothing recorded: Docker's ID, quietly.
	tl.docker["myapp:dev"] = digestOf("d")
	if err := tl.Refresh(t.Context(), "myapp:dev"); err != nil || len(tl.warned.Names()) != 0 {
		t.Fatalf("Refresh: %v, warnings %q", err, tl.warned.Names())
	}
	if d := tl.recorded(t, "myapp:dev"); d != digestOf("d") {
		t.Errorf("recorded %q", d)
	}
	// A digest seen online stays, with a warning: base isn't rebuilt from
	// Docker's copy.
	tl.docker["node:24"] = digestOf("e")
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, "node:24"); d != "node@"+digestOf("a") {
		t.Errorf("recorded %q", d)
	}
	if want := "from_cached_unreachable from=node:24 host=" + DockerHub; strings.Join(tl.warned.Names(), "\n") != want {
		t.Errorf("warnings %q, want %q", tl.warned.Names(), want)
	}
	// Neither: an error.
	if err := tl.Refresh(t.Context(), "debian:13"); msg.IDOf(err) != "from_unreachable" {
		t.Errorf("debian:13: %v", err)
	}
}

func TestRefreshNotFound(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	if err := tl.Refresh(t.Context(), "ghcr.io/someone/private"); msg.IDOf(err) != "from_not_found" {
		t.Errorf("nothing recorded: %v", err)
	}
	if err := state.SetFromDigest(tl.Dirs, "ghcr.io/someone/private", digestOf("f")); err != nil {
		t.Fatal(err)
	}
	if err := tl.Refresh(t.Context(), "ghcr.io/someone/private"); err != nil {
		t.Fatal(err)
	}
	if want := "from_cached_not_found from=ghcr.io/someone/private host=ghcr.io"; strings.Join(tl.warned.Names(), "\n") != want {
		t.Errorf("warnings %q, want %q", tl.warned.Names(), want)
	}
}

func TestRefreshPinnedNeedsNoLookup(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	ref := "node:24@sha256:cccc"
	if err := tl.Refresh(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if d := tl.recorded(t, ref); d != ref {
		t.Errorf("Recorded = %q", d)
	}
	if len(tl.reg.calls) != 0 || tl.dockerCalls != 0 {
		t.Errorf("looked up: %q, docker %d", tl.reg.calls, tl.dockerCalls)
	}
}

// Public pins an image its registry gives anyone, and never asks Docker.
func TestPublic(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	tl.reg.set("library/node:24", digestOf("a"))
	tl.reg.set("library/node:"+digestOf("b"), digestOf("b"))
	tl.docker["myapp:dev"] = digestOf("d")
	got, err := tl.Public(t.Context(), "node:24")
	if err != nil || got != "node@"+digestOf("a") {
		t.Fatalf("Public = %q, %v", got, err)
	}
	if d := tl.recorded(t, "node:24"); d != got {
		t.Errorf("recorded %q", d)
	}
	// A pinned reference is asked for by its digest.
	if got, err := tl.Public(t.Context(), "node@"+digestOf("b")); err != nil || got != "node@"+digestOf("b") {
		t.Errorf("pinned: %q, %v", got, err)
	}
	if _, err := tl.Public(t.Context(), "node@"+digestOf("c")); !errors.Is(err, ErrNotFound) {
		t.Errorf("a pinned digest the registry hasn't: %v", err)
	}
	// Only Docker has it: not public.
	if _, err := tl.Public(t.Context(), "myapp:dev"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an image only Docker has: %v", err)
	}
	if tl.dockerCalls != 0 {
		t.Errorf("docker asked %d times", tl.dockerCalls)
	}
	// Offline: the registry's digest seen before, with a warning; Docker's
	// ID recorded for another build isn't one.
	if err := state.SetFromDigest(tl.Dirs, "myapp:dev", digestOf("d")); err != nil {
		t.Fatal(err)
	}
	tl.offline(t)
	if got, err := tl.Public(t.Context(), "node:24"); err != nil || got != "node@"+digestOf("a") {
		t.Errorf("offline: %q, %v", got, err)
	}
	if want := "from_cached_unreachable from=node:24 host=" + DockerHub; strings.Join(tl.warned.Names(), "\n") != want {
		t.Errorf("warnings %q, want %q", tl.warned.Names(), want)
	}
	for _, ref := range []string{"myapp:dev", "debian:13", "node@" + digestOf("b")} {
		if _, err := tl.Public(t.Context(), ref); msg.IDOf(err) != "from_unreachable" {
			t.Errorf("offline %s: %v", ref, err)
		}
	}
}

// A registry on this machine or its network is never public, by its
// name: never asked, and a digest recorded for it (a --from of yours)
// never stands in for its answer.
func TestPublicRefusesLocalRegistries(t *testing.T) {
	t.Parallel()
	tl := newTestLookup(t)
	refs := []string{
		"localhost:5000/app:1", "localhost/app", "reg.localhost:5000/app", "127.0.0.1:5000/app",
		"[::1]:5000/app", "10.0.0.5/app:1", "192.168.1.2:5000/app", "169.254.169.254/app", "100.96.0.1:5000/app",
		"localhost:5000/app@" + digestOf("a"),
	}
	for _, ref := range refs {
		tl.reg.set(ParseRef(ref).Repo+":"+ParseRef(ref).Tag, digestOf("a"))
		if err := state.SetFromDigest(tl.Dirs, ref, Name(ref)+"@"+digestOf("a")); err != nil {
			t.Fatal(err)
		}
	}
	for _, offline := range []bool{false, true} {
		if offline {
			tl.offline(t)
		}
		for _, ref := range refs {
			if got, err := tl.Public(t.Context(), ref); !errors.Is(err, ErrNotFound) {
				t.Errorf("%s (offline %v) = %q, %v", ref, offline, got, err)
			}
		}
	}
	if len(tl.reg.calls) != 0 || len(tl.warned.Names()) != 0 {
		t.Errorf("asked %q, warned %q", tl.reg.calls, tl.warned.Names())
	}
}
