package backends

import (
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

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
