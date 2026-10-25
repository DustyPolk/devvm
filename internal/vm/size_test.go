package vm

import (
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/state"
)

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
	var b strings.Builder
	if err := e.ShowSize(t.Context(), &b, false); names(err) != "no_vm_size dir="+e.Dir {
		t.Errorf("no VM: %s", names(err))
	}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	in := f.Instances[vm]
	in.Size = backend.Size{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}
	f.Instances[vm] = in

	before := len(f.Calls())
	if err := e.SetSize(t.Context(), Size{CPUs: "2", Memory: "8G"}, false); err != nil {
		t.Fatal(names(err))
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

	if err := e.SetSize(t.Context(), Size{Disk: "50"}, false); names(err) != "disk_shrink have=100 vm="+vm+" want=50" {
		t.Errorf("smaller disk: %s", names(err))
	}
	caps := allNative()
	caps[backend.FeatureDisk] = backend.None
	f.Capabilities = caps
	if err := e.SetSize(t.Context(), Size{Disk: "200"}, false); !strings.HasPrefix(names(err), "backend_lacks ") {
		t.Errorf("without the feature: %s", names(err))
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(KeyDisk) != "" {
		t.Errorf("disk=%q", st.Get(KeyDisk))
	}
	f.Capabilities = allNative()

	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := e.SetSize(t.Context(), Size{Disk: "120"}, false); err != nil {
		t.Fatal(names(err))
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
	b.Reset()
	if err := e.ShowSize(t.Context(), &b, false); err != nil || b.String() != "cpus=2 memory=8GiB disk=120GiB\n" {
		t.Errorf("ShowSize = %q, %v", b.String(), names(err))
	}

	if err := e.SetSize(t.Context(), Size{CPUs: "2", Memory: "3"}, true); err != nil {
		t.Fatal(names(err))
	}
	if err := e.SetSize(t.Context(), Size{Memory: "4GiB"}, true); err != nil {
		t.Fatal(names(err))
	}
	if err := e.SetSize(t.Context(), Size{CPUs: "16"}, true); names(err) != "too_many_cpus have=8 want=16" {
		t.Errorf("-g --cpus 16: %s", names(err))
	}
	b.Reset()
	if err := e.ShowSize(t.Context(), &b, true); err != nil || b.String() != "cpus=2\nmemory=4\ndisk=\n" {
		t.Errorf("ShowSize -g = %q, %v", b.String(), names(err))
	}
}
