package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/proxy"
)

// devvm 0.1.0's store on Linux (testdata/v0.1.0/linux, which 0.1.0
// itself made), where the github secret is a file, gives app's proxy the
// value its github binding asks for: the binding 0.1.0 wrote, without the
// ID a secret of yours has, still fits the secret as it's defined today.
// The copy is the test's own, its files private as 0.1.0 left them (the
// fixture's modes, which git doesn't keep).
func TestV010StoreGet(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "v0.1.0", "linux")
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	d := paths.Dirs{
		Home: filepath.Join(dst, "home"), Data: filepath.Join(dst, "data", "devvm"),
		Config: filepath.Join(dst, "config", "devvm"),
	}
	bindings := filepath.Join(d.Data, "vms", "6a14d8553d070e", "run", "secrets.json")
	for _, f := range []string{filepath.Join(d.Secrets(), "github"), bindings} {
		if err := os.Chmod(f, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	list, err := proxy.ReadBindings(bindings)
	if err != nil {
		t.Fatalf("ReadBindings(app) = %v", err)
	}
	s := NewStore(d, &msg.Fake{OnWarn: func(m msg.M) { t.Log(m.Names()) }})
	s.Keychain = false
	s.Getenv = func(string) string { return "" }
	var found bool
	for _, b := range list.Bindings {
		if b.Name != "github" {
			continue
		}
		found = true
		if v, err := s.Get(t.Context(), b); v != "ghp_testfakev010" || err != nil {
			t.Errorf("Get(%+v) = %q, %v", b, v, err)
		}
	}
	if !found {
		t.Fatalf("app's bindings have no github: %+v", list.Bindings)
	}
}
