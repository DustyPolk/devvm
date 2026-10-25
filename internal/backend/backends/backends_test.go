package backends

import (
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// DEVVM_UNREVIEWED counts only in a test build: a release ignores it.
func TestUnreviewedOnlyInTestBuilds(t *testing.T) {
	t.Setenv("DEVVM_UNREVIEWED", "smolvm:egress-proxy")
	t.Cleanup(func() { unreviewed, backend.Unreviewed = "", nil })
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	for _, c := range []struct {
		build string
		want  bool
	}{{"", false}, {"yes", false}, {"allowed", true}} {
		unreviewed = c.build
		if _, err := Registry(d); err != nil {
			t.Fatal(err)
		}
		if got := backend.Unreviewed["smolvm:egress-proxy"]; got != c.want {
			t.Errorf("build %q: let through = %v", c.build, got)
		}
	}
}

// lima is registered, and it's what a new VM gets by default.
func TestLimaIsTheDefault(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	r, err := Registry(d)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("lima"); !ok {
		t.Fatalf("no lima in %q", r.Names())
	}
	vm, err := state.ReadVM(d, "devvm-p-0123456789abcd")
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.For(d, vm, "")
	if err != nil || c.Backend.Name() != backend.Default || backend.Default != "lima" {
		t.Errorf("For a new VM: %+v, %v", c, err)
	}
}
