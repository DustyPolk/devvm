package clipboard

import (
	"testing"

	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/state"
)

func TestMode(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir()}
	st, err := state.New(d, "devvm-web-0123456789abcd")
	if err != nil {
		t.Fatal(err)
	}
	st.Image, st.Dir, st.Backend = "base", "/p", "lima"
	if err := st.Create(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if got := Mode(st); got != Off {
		t.Errorf("a VM without the section: %q, want off", got)
	}
	for _, mode := range []string{Images, Off, Images} {
		if err := st.Update(t.Context(), func(st *state.VM) error { return SetMode(st, mode) }); err != nil {
			t.Fatal(err)
		}
		again, err := state.ReadVM(d, st.Name())
		if err != nil {
			t.Fatal(err)
		}
		if got := Mode(again); got != mode {
			t.Errorf("after SetMode(%q), Mode = %q", mode, got)
		}
	}
	if err := st.Update(t.Context(), func(st *state.VM) error { return SetMode(st, "text") }); err == nil {
		t.Error("SetMode(text) took a mode devvm doesn't have")
	}
	if got := Mode(st); got != Images {
		t.Errorf("after a refused SetMode, Mode = %q, want images", got)
	}
	// A section devvm didn't write reads as off.
	if err := st.Update(t.Context(), func(st *state.VM) error {
		return state.SetSection(st, Section, InVM{Mode: "everything"})
	}); err != nil {
		t.Fatal(err)
	}
	if got := Mode(st); got != Off {
		t.Errorf("an unknown mode in vm.json: %q, want off", got)
	}
	if err := st.Update(t.Context(), func(st *state.VM) error {
		return state.SetSection(st, Section, map[string]int{"mode": 1})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Of(st); err == nil {
		t.Error("Of read a section that isn't one")
	}
	if got := Mode(st); got != Off {
		t.Errorf("an unreadable section: %q, want off", got)
	}
}
