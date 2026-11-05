package vm

import (
	"reflect"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// A target holds its VM as the backend answered the finder, and each
// start and stop of it then sets what it made the VM, a size its state
// asks for included, as the backend has it then: a start reads it once,
// as it begins, and a stop not at all.
func TestTargetAfterStartAndStop(t *testing.T) {
	e, f := startEnv(t, allNative())
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	stopVM(t, f, st)
	if err := (Settings{e}).SetSize(t.Context(), Size{CPUs: "2"}, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	tg, err := e.finder.target(t.Context(), need{has: hasMade})
	if err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if tg.in.Status != backend.StatusStopped || tg.in.Size.CPUs == 2 {
		t.Fatalf("found %+v", tg.in)
	}
	for _, step := range []struct {
		name     string
		do       func() error
		status   backend.Status
		inspects int
	}{
		{"start", func() error { return tg.ensureRunning(t.Context()) }, backend.StatusRunning, 1},
		{"stop", func() error { return tg.stop(t.Context()) }, backend.StatusStopped, 0},
	} {
		before := len(f.Calls())
		if err := step.do(); err != nil {
			t.Fatalf("%s: %s", step.name, msg.NamesOf(err))
		}
		if n := countOf(f.Methods()[before:], "Inspect"); n != step.inspects {
			t.Errorf("%s: %d inspects, want %d", step.name, n, step.inspects)
		}
		if have := f.Instances[vm]; tg.in.Status != step.status || !reflect.DeepEqual(tg.in, have) {
			t.Errorf("%s: target has %+v, the backend %+v", step.name, tg.in, have)
		}
	}
	if tg.in.Size.CPUs != 2 {
		t.Errorf("size %+v, want its state's 2 CPUs", tg.in.Size)
	}
}
