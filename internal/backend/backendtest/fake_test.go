package backendtest

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
)

func TestFakeRecordsAndAnswers(t *testing.T) {
	ctx := context.Background()
	f := New("fake", backend.Caps{backend.FeatureClone: backend.Native})
	f.ShellCmd = backend.Command{Path: "/bin/true", Args: []string{"true"}}

	if _, err := f.BuildImage(ctx, backend.ImageSpec{Image: "base", Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := f.CreateVM(ctx, "k1", backend.VMSpec{Name: "v", Size: backend.Size{CPUs: 2}}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSize(ctx, "v", backend.Size{MemoryGiB: 4}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetAgentDirs(ctx, "v", []string{"/h/.claude"}); err != nil {
		t.Fatal(err)
	}
	if err := f.Start(ctx, "v", backend.Confinement{Net: backend.NetProxy}); err != nil {
		t.Fatal(err)
	}
	in, err := f.Inspect(ctx, "v")
	if err != nil {
		t.Fatal(err)
	}
	if in.Status != backend.StatusRunning || in.Size != (backend.Size{CPUs: 2, MemoryGiB: 4}) || !slices.Equal(in.AgentDirs, []string{"/h/.claude"}) {
		t.Errorf("instance: %+v", in)
	}
	if c, err := f.Shell(ctx, "v", "/p"); err != nil || c.Path != "/bin/true" {
		t.Errorf("Shell: %+v, %v", c, err)
	}
	if err := f.Delete(ctx, "v"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Inspect(ctx, "v"); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("Inspect after Delete: %v", err)
	}
	if err := f.Stop(ctx, "v"); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("Stop after Delete: %v", err)
	}
	want := []string{"BuildImage", "CreateVM", "SetSize", "SetAgentDirs", "Start", "Inspect", "Shell", "Delete", "Inspect", "Stop"}
	if got := f.Methods(); !slices.Equal(got, want) {
		t.Errorf("calls: %q, want %q", got, want)
	}
	if c := f.Calls()[4]; c.Args[0] != "v" || c.Args[1].(backend.Confinement).Net != backend.NetProxy {
		t.Errorf("Start's arguments: %v", c.Args)
	}
}

func TestFakePresetErrors(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	f := New("fake", nil)
	f.Errs = map[string]error{"CreateVM": boom}
	if err := f.CreateVM(ctx, "k1", backend.VMSpec{Name: "v"}); !errors.Is(err, boom) {
		t.Errorf("CreateVM: %v", err)
	}
	if ins, err := f.List(ctx); err != nil || len(ins) != 0 {
		t.Errorf("a failed CreateVM made a VM: %v, %v", ins, err)
	}
}
