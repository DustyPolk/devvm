package vm

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

func TestParseSizes(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, tc := range []struct{ in, want, err string }{
		{"6", "6", ""},
		{"1.5", "", `cpus_not_number value="1.5"`},
		{"0", "", `cpus_not_number value="0"`},
		{"many", "", `cpus_not_number value="many"`},
	} {
		got, err := ParseCPUs(tc.in)
		if got != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("ParseCPUs(%q) = %q, %v", tc.in, got, msg.NamesOf(err))
		}
	}
	for _, tc := range []struct{ in, want, err string }{
		{"16", "16", ""},
		{"16G", "16", ""},
		{"16g", "16", ""},
		{"200GiB", "200", ""},
		{"1.5", "1.5", ""},
		{"0.5", "0.5", ""},
		{"0", "", `size_not_gib flag=--memory value="0"`},
		{"16M", "", `size_not_gib flag=--memory value="16M"`},
		{"lots", "", `size_not_gib flag=--memory value="lots"`},
		{"1$(touch x)", "", `size_not_gib flag=--memory value="1$(touch x)"`},
	} {
		got, err := ParseGiB("--memory", tc.in)
		if got != tc.want || msg.NamesOf(err) != tc.err {
			t.Errorf("ParseGiB(%q) = %q, %v", tc.in, got, msg.NamesOf(err))
		}
	}
	s, err := Size{CPUs: "6", Memory: "12G", Disk: "150"}.Parse()
	if err != nil || s != (Size{CPUs: "6", Memory: "12", Disk: "150"}) {
		t.Errorf("Parse = %+v, %v", s, err)
	}
	if _, err := (Size{Disk: "x"}).Parse(); msg.NamesOf(err) != `size_not_gib flag=--disk value="x"` {
		t.Errorf("Parse with a bad disk: %v", msg.NamesOf(err))
	}
}

// The flags, else the defaults, else nothing (the image's).
func TestSizeOr(t *testing.T) {
	got := Size{CPUs: "6"}.Or(Size{CPUs: "2", Memory: "16"})
	if got != (Size{CPUs: "6", Memory: "16"}) {
		t.Errorf("Or = %+v", got)
	}
}

func TestSizeCheck(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	h := Host{CPUs: 8, MemoryGiB: 32}
	for _, tc := range []struct {
		s   Size
		err string
	}{
		{Size{CPUs: "16"}, "too_many_cpus have=8 want=16"},
		{Size{Memory: "64"}, "too_much_memory have=32 max=30 want=64"},
		{Size{Memory: "32"}, "too_much_memory have=32 max=30 want=32"},
		{Size{Memory: "30.5"}, "too_much_memory have=32 max=30 want=30.5"},
		{Size{Disk: "50"}, "disk_shrink have=100 vm=v want=50"},
		{Size{CPUs: "8", Memory: "30", Disk: "200"}, ""},
		{Size{}, ""},
	} {
		if err := tc.s.Check(h, 100, "v"); msg.NamesOf(err) != tc.err {
			t.Errorf("Check(%+v) = %v, want %s", tc.s, msg.NamesOf(err), tc.err)
		}
	}
	// The memory a VM may have keeps 2 GiB for the host, or half of it on
	// a host of 4 GiB or less.
	for _, tc := range []struct {
		host int
		mem  string
		err  string
	}{
		{8, "8", "too_much_memory have=8 max=6 want=8"},
		{8, "7", "too_much_memory have=8 max=6 want=7"},
		{8, "6", ""},
		{4, "2", ""},
		{4, "3", "too_much_memory have=4 max=2 want=3"},
		{3, "1.5", ""},
		{3, "2", "too_much_memory have=3 max=1.5 want=2"},
	} {
		if err := (Size{Memory: tc.mem}).Check(Host{CPUs: 8, MemoryGiB: tc.host}, 0, "v"); msg.NamesOf(err) != tc.err {
			t.Errorf("Check(memory %s on %d GiB) = %v, want %q", tc.mem, tc.host, msg.NamesOf(err), tc.err)
		}
	}
	if b := (Size{CPUs: "2", Memory: "1.5", Disk: "120"}).Backend(); b != (backend.Size{CPUs: 2, MemoryGiB: 1.5, DiskGiB: 120}) {
		t.Errorf("Backend = %+v", b)
	}
}

func TestBaseDisk(t *testing.T) {
	if got := BaseDisk(assets.FS); got != 100 {
		t.Errorf("BaseDisk(devvm's files) = %v, want base.yaml's 100", got)
	}
	if got := BaseDisk(fstest.MapFS{}); got != 0 {
		t.Errorf("BaseDisk(no base.yaml) = %v", got)
	}
}

func TestSizeText(t *testing.T) {
	for s, want := range map[backend.Size]string{
		{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}: "cpus=4 memory=8GiB disk=100GiB",
		{MemoryGiB: 1.5}:                      "memory=1.5GiB",
		{}:                                    "",
	} {
		if got := SizeText(s); got != want {
			t.Errorf("SizeText(%+v) = %q, want %q", s, got, want)
		}
	}
}

// devvm size: a stopped VM's size changes at once, only where it differs;
// a running one's waits, and shows as pending. A smaller disk, or a size
// the backend can't set, is refused before the state changes. -g changes
// the size file and keeps what it leaves unset.
func TestSetSize(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	s := Settings{e}
	shown := func(global bool) (string, error) {
		got, err := s.ShowSize(t.Context(), global)
		return got.Text(), err
	}
	if _, err := shown(false); msg.NamesOf(err) != "no_vm_size dir="+e.Dir {
		t.Errorf("no VM: %s", msg.NamesOf(err))
	}
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	in := f.Instances[vm]
	in.Size = backend.Size{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}
	f.Instances[vm] = in

	before := len(f.Calls())
	if err := s.SetSize(t.Context(), Size{CPUs: "2", Memory: "8G"}, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if ms := f.Methods()[before:]; slices.Contains(ms, "SetSize") {
		t.Errorf("a running VM was changed: %q", ms)
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(KeyCPUs) != "2" || st.Get(KeyMemory) != "8" {
		t.Errorf("state cpus=%q memory=%q", st.Get(KeyCPUs), st.Get(KeyMemory))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if p := pendingSize(st, f.Instances[vm].Size); p != (backend.Size{CPUs: 2}) {
		t.Errorf("pending = %+v", p)
	}

	if err := s.SetSize(t.Context(), Size{Disk: "50"}, false); msg.NamesOf(err) != "disk_shrink have=100 vm="+vm+" want=50" {
		t.Errorf("smaller disk: %s", msg.NamesOf(err))
	}
	caps := allNative()
	caps[backend.FeatureDisk] = backend.None
	f.Capabilities = caps
	if err := s.SetSize(t.Context(), Size{Disk: "200"}, false); !strings.HasPrefix(msg.NamesOf(err), "backend_lacks ") {
		t.Errorf("without the feature: %s", msg.NamesOf(err))
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(KeyDisk) != "" {
		t.Errorf("disk=%q", st.Get(KeyDisk))
	}
	f.Capabilities = allNative()

	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := s.SetSize(t.Context(), Size{Disk: "120"}, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	var set []backend.Size
	for _, c := range f.Calls()[before:] {
		if c.Method == "SetSize" {
			set = append(set, c.Args[1].(backend.Size))
		}
	}
	if len(set) != 1 || set[0] != (backend.Size{CPUs: 2, DiskGiB: 120}) {
		t.Errorf("SetSize calls = %+v", set)
	}
	if got, err := shown(false); err != nil || got != "cpus=2 memory=8GiB disk=120GiB\n" {
		t.Errorf("ShowSize = %q, %v", got, msg.NamesOf(err))
	}

	if err := s.SetSize(t.Context(), Size{CPUs: "2", Memory: "3"}, true); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := s.SetSize(t.Context(), Size{Memory: "4GiB"}, true); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := s.SetSize(t.Context(), Size{CPUs: "16"}, true); msg.NamesOf(err) != "too_many_cpus have=8 want=16" {
		t.Errorf("-g --cpus 16: %s", msg.NamesOf(err))
	}
	if got, err := shown(true); err != nil || got != "cpus=2\nmemory=4\ndisk=\n" {
		t.Errorf("ShowSize -g = %q, %v", got, msg.NamesOf(err))
	}
}

// diskFake is a fake backend whose VMs' disks have no size of their own
// (backend.DiskUser): each holds used bytes, against limit.
type diskFake struct {
	*backendtest.Fake
	used, limit int64
}

func (d *diskFake) DiskUsed(context.Context, string) (int64, int64, error) {
	return d.used, d.limit, nil
}

// On a backend that sizes no disks, devvm size shows what the VM's disk
// holds, and it and each launch warn once that's past the size VMs get
// elsewhere.
func TestDiskUsed(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDisk)
	e, f, _ := upEnv(t, caps)
	df := &diskFake{Fake: f, used: 3 << 29, limit: 100 << 30}
	reg, err := backend.NewRegistry(df)
	if err != nil {
		t.Fatal(err)
	}
	e.Registry = reg
	var warned msg.Recorder
	e.Warn = warned.Add
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Ports: "none"})
	in := f.Instances[st.Name()]
	in.Size = backend.Size{CPUs: 4, MemoryGiB: 8}
	f.Instances[st.Name()] = in
	s, err := (Settings{e}).ShowSize(t.Context(), false)
	if err != nil || s.Text() != "cpus=4 memory=8GiB disk_used=1.5GiB\n" || !s.DiskMeasured || s.DiskUsedGiB != 1.5 || len(warned.Names()) != 0 {
		t.Errorf("ShowSize = %+v, %v, warned %q", s, msg.NamesOf(err), warned.Names())
	}
	df.used = 101 << 30
	if s, err := (Settings{e}).ShowSize(t.Context(), false); err != nil || s.Text() != "cpus=4 memory=8GiB disk_used=101GiB\n" {
		t.Errorf("ShowSize = %q, %v", s.Text(), msg.NamesOf(err))
	}
	if w := warned.Names(); len(w) != 1 || w[0] != "disk_over backend=fake limit=100 used=101 vm="+st.Name() {
		t.Errorf("warned %q", w)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	w := warned.Names()
	if len(w) != 2 || w[1] != w[0] {
		t.Errorf("a launch warned %q", w)
	}
	ls, err := List(t.Context(), e.Dirs, e.Registry)
	if err != nil || len(ls) != 1 || ls[0].DiskOver == nil || ls[0].DiskOver.Names() != w[0] {
		t.Errorf("List = %+v, %v", ls, err)
	}
	df.used = 1 << 30
	if ls, err := List(t.Context(), e.Dirs, e.Registry); err != nil || len(ls) != 1 || ls[0].DiskOver != nil {
		t.Errorf("List under the limit = %+v, %v", ls, err)
	}
}
