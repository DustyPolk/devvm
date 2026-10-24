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
	for _, s := range []string{"node:24", "ghcr.io/o/i@sha256:0123", "localhost:5000/a_b/c-d:1.2+x"} {
		if !Valid(s) || Check(s) != nil {
			t.Errorf("%q refused", s)
		}
	}
	for _, s := range []string{"node:24\nimage=x", "-x", " node", "node 24", "a;b", "$(x)", "a\x1b[2J"} {
		if Valid(s) {
			t.Errorf("%q accepted", s)
		}
		if m, ok := errors.AsType[msg.M](Check(s)); !ok || m.ID() != "bad_from" {
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
	warned      []string
}

func newTestLookup(t *testing.T) *testLookup {
	t.Helper()
	dir := t.TempDir()
	tl := &testLookup{reg: newFakeRegistry(t), docker: map[string]string{}}
	tl.Lookup = &Lookup{
		Dirs:     paths.Dirs{Data: filepath.Join(dir, "data"), Config: filepath.Join(dir, "config")},
		Registry: tl.reg.registry(),
		DockerID: func(_ context.Context, ref string) (string, bool) {
			tl.dockerCalls++
			id, ok := tl.docker[ref]
			return id, ok
		},
		Warn: func(m msg.M) { tl.warned = append(tl.warned, m.Names()) },
	}
	return tl
}

// offline makes the registry unreachable.
func (tl *testLookup) offline(t *testing.T) {
	t.Helper()
	retryDelay = 0
	down := httptest.NewTLSServer(http.NotFoundHandler())
	tl.Registry = &Registry{Client: down.Client(), Base: func(string) string { return down.URL }}
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

func isMsg(err error, id string) bool {
	m, ok := errors.AsType[msg.M](err)
	return ok && m.ID() == id
}

func TestRefreshRecordsTheRegistrysDigestPerTag(t *testing.T) {
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
	if tl.dockerCalls != 0 || len(tl.warned) != 0 {
		t.Errorf("docker asked %d times, warnings %q", tl.dockerCalls, tl.warned)
	}
}

func TestRefreshDockerOnlyImage(t *testing.T) {
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
	tl := newTestLookup(t)
	tl.reg.set("library/node:24", digestOf("a"))
	if err := tl.Refresh(t.Context(), "node:24"); err != nil {
		t.Fatal(err)
	}
	tl.offline(t)
	// Nothing recorded: Docker's ID, quietly.
	tl.docker["myapp:dev"] = digestOf("d")
	if err := tl.Refresh(t.Context(), "myapp:dev"); err != nil || len(tl.warned) != 0 {
		t.Fatalf("Refresh: %v, warnings %q", err, tl.warned)
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
	if want := "from_cached_unreachable from=node:24 host=" + DockerHub; strings.Join(tl.warned, "\n") != want {
		t.Errorf("warnings %q, want %q", tl.warned, want)
	}
	// Neither: an error.
	if err := tl.Refresh(t.Context(), "debian:13"); !isMsg(err, "from_unreachable") {
		t.Errorf("debian:13: %v", err)
	}
}

func TestRefreshNotFound(t *testing.T) {
	tl := newTestLookup(t)
	if err := tl.Refresh(t.Context(), "ghcr.io/someone/private"); !isMsg(err, "from_not_found") {
		t.Errorf("nothing recorded: %v", err)
	}
	if err := state.SetFromDigest(tl.Dirs, "ghcr.io/someone/private", digestOf("f")); err != nil {
		t.Fatal(err)
	}
	if err := tl.Refresh(t.Context(), "ghcr.io/someone/private"); err != nil {
		t.Fatal(err)
	}
	if want := "from_cached_not_found from=ghcr.io/someone/private host=ghcr.io"; strings.Join(tl.warned, "\n") != want {
		t.Errorf("warnings %q, want %q", tl.warned, want)
	}
}

func TestRefreshPinnedNeedsNoLookup(t *testing.T) {
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
