package backendtest

import (
	"errors"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

func TestFakeRecordsAndAnswers(t *testing.T) {
	ctx := t.Context()
	f := New("fake", backend.Caps{backend.FeatureClone: backend.Native})
	f.ShellCmd = backend.Command{Path: "/bin/true", Args: []string{"true"}}

	if _, err := f.BuildImage(ctx, backend.ImageSpec{Image: "base", Key: "k1"}); err != nil {
		t.Fatal(err)
	}
	if err := f.CreateVM(ctx, "k1", backend.VMSpec{Name: "v", Want: backend.Want{Size: backend.Size{CPUs: 2}}}); err != nil {
		t.Fatal(err)
	}
	// A start makes a stopped VM what it wants; a running one stays.
	w := backend.Want{Size: backend.Size{MemoryGiB: 4}, AgentDirs: []string{"/h/.claude"}}
	if err := f.Start(ctx, "v", w, backend.Confinement{Net: backend.NetProxy}); err != nil {
		t.Fatal(err)
	}
	if err := f.Start(ctx, "v", backend.Want{}, backend.Confinement{Net: backend.NetProxy}); err != nil {
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
	want := []string{"BuildImage", "CreateVM", "Start", "Start", "Inspect", "Shell", "Delete", "Inspect", "Stop"}
	if got := f.Methods(); !slices.Equal(got, want) {
		t.Errorf("calls: %q, want %q", got, want)
	}
	if c := f.Calls()[2]; c.Args[0] != "v" || c.Args[2].(backend.Confinement).Net != backend.NetProxy || c.Args[1].(backend.Want).Size.MemoryGiB != 4 {
		t.Errorf("Start's arguments: %v", c.Args)
	}
}

// A stopped VM's start refuses a disk smaller than its own, changing
// nothing.
// Running gives the lifeline, the start's ID and its confinement; the
// roles Lima has and the fake doesn't answer none.
func TestFakeRunningAndNone(t *testing.T) {
	ctx := t.Context()
	f := New("fake", nil)
	f.LifelineFile = "/ha.pid"
	f.Instances["v"] = backend.Instance{Name: "v", Status: backend.StatusStopped}
	if r, err := f.Running(ctx, "v"); r != (backend.Run{Lifeline: "/ha.pid", Confined: true}) || err != nil {
		t.Errorf("stopped: %+v, %v", r, err)
	}
	if err := f.Start(ctx, "v", backend.Want{}, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	if r, err := f.Running(ctx, "v"); r != (backend.Run{ID: "start-1", Lifeline: "/ha.pid"}) || err != nil {
		t.Errorf("started direct: %+v, %v", r, err)
	}
	f.Restart("v")
	if r, _ := f.Running(ctx, "v"); r.ID != "start-2" {
		t.Errorf("restarted: %+v", r)
	}
	where, isDefault := f.Location()
	u, uerr := f.Usage(ctx)
	ss, serr := f.Strays(ctx)
	cs, cerr := f.DevvmCache(ctx)
	if where != "" || !isDefault || f.DiskDirs() != nil || f.Floors() != (backend.Floors{}) || f.KeyFiles() != nil ||
		u.VMs != nil || uerr != nil || ss != nil || serr != nil || cs != nil || cerr != nil {
		t.Error("the fake has something of a role it has none of")
	}
	if err := f.DeleteCache(ctx, nil); err != nil || !slices.Contains(f.Methods(), "DeleteCache") {
		t.Errorf("DeleteCache: %v, %q", err, f.Methods())
	}
}

func TestFakeStartValidates(t *testing.T) {
	ctx := t.Context()
	f := New("fake", nil)
	f.Instances["v"] = backend.Instance{Name: "v", Status: backend.StatusStopped, Size: backend.Size{DiskGiB: 100}}
	err := f.Start(ctx, "v", backend.Want{Size: backend.Size{DiskGiB: 50}}, backend.Confinement{})
	if msg.NamesOf(err) != "disk_shrink have=100 vm=v want=50" || f.Instances["v"].Status != backend.StatusStopped {
		t.Errorf("Start: %v, %+v", err, f.Instances["v"])
	}
}

func TestFakePresetErrors(t *testing.T) {
	ctx := t.Context()
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
