package registry

import (
	"context"
	"os/exec"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// quietUI prints nothing, runs commands as they are, counts no steps and
// keeps no cleanups.
type quietUI struct{}

func (quietUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }
func (quietUI) Info(msg.M)                                                {}
func (quietUI) Note(msg.M)                                                {}
func (quietUI) Warn(msg.M)                                                {}
func (quietUI) LogNote()                                                  {}
func (quietUI) WithSteps(ctx context.Context, _ int) context.Context      { return ctx }
func (quietUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (quietUI) Interrupted() bool                                         { return false }
func (quietUI) StdinIsTerminal() bool                                     { return false }

// DEVVM_UNREVIEWED counts only in a test build: a release ignores it.
func TestUnreviewedOnlyInTestBuilds(t *testing.T) {
	t.Parallel()
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	for _, c := range []struct {
		build string
		want  bool
	}{{"", false}, {"yes", false}, {"allowed", true}} {
		r, err := newFor(d, assets.FS, quietUI{}, c.build, "smolvm:egress-proxy")
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Unreviewed["smolvm:egress-proxy"]; got != c.want {
			t.Errorf("build %q: let through = %v", c.build, got)
		}
	}
}

// lima is registered, and it's what a new VM gets by default.
func TestLimaIsTheDefault(t *testing.T) {
	t.Parallel()
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	r, err := New(d, assets.FS, quietUI{})
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
