package backend_test

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

func testDirs(t *testing.T) paths.Dirs {
	t.Helper()
	tmp := t.TempDir()
	return paths.Dirs{
		Data:   filepath.Join(tmp, "data", "devvm"),
		Config: filepath.Join(tmp, "config", "devvm"),
		Lima:   filepath.Join(tmp, "lima"),
	}
}

// newState writes the state of a VM named name, made on backend b.
func newState(t *testing.T, d paths.Dirs, name, b string) {
	t.Helper()
	st, err := state.New(d, name)
	if err != nil {
		t.Fatal(err)
	}
	st.Image, st.Dir, st.Backend = "base", "/p", b
	if err := st.Create(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func testRegistry(t *testing.T, names ...string) *backend.Registry {
	t.Helper()
	var bs []backend.Backend
	for _, n := range names {
		bs = append(bs, backendtest.New(n, nil))
	}
	r, err := backend.NewRegistry(bs...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRegistry(t *testing.T) {
	r := testRegistry(t, "vz", "lima")
	if got := r.Names(); len(got) != 2 || got[0] != "lima" || got[1] != "vz" {
		t.Errorf("Names: %q", got)
	}
	if all := r.All(); len(all) != 2 || all[0].Name() != "lima" || all[1].Name() != "vz" {
		t.Errorf("All: %v", all)
	}
	if b, ok := r.Lookup("vz"); !ok || b.Name() != "vz" {
		t.Errorf("Lookup(vz): %v, %v", b, ok)
	}
	if _, ok := r.Lookup("fake"); ok {
		t.Error("Lookup(fake): found")
	}
	for _, name := range []string{"lima", "", "Lima", "a b", "a=b", "a/b"} {
		if err := r.Register(backendtest.New(name, nil)); err == nil {
			t.Errorf("Register(%q): want an error", name)
		}
	}
}

func TestFor(t *testing.T) {
	for _, tc := range []struct {
		name     string
		newVMs   string // the registry's NewVMs; "" for Default
		recorded string // the VM's backend=; "" for a new VM
		want     string // the backend chosen, or the error's names
	}{
		{name: "a new VM", want: "lima"},
		{name: "a new VM, NewVMs set", newVMs: "vz", want: "vz"},
		{name: "an existing VM keeps its backend", newVMs: "fake", recorded: "vz", want: "vz"},
		{name: "an existing VM on a backend not here", recorded: "qemu", want: "backend_unknown_vm backend=qemu vm=devvm-x-0123456789abcd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			r := testRegistry(t, "lima", "vz", "fake")
			r.NewVMs = tc.newVMs
			vm := readVM(t, d, tc.recorded)
			c, err := r.For(vm)
			if err != nil {
				if got := msg.NamesOf(err); got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
				return
			}
			if c.Backend.Name() != tc.want {
				t.Errorf("got %s, want %s", c.Backend.Name(), tc.want)
			}
		})
	}
}

// Used is the registered backends some VM's state names, sorted: a state
// without backend= and one naming a backend not here count for none.
func TestUsed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vms   []string // each VM's backend
		want  string
		files string // a file that isn't a VM's state, in their folder
	}{
		{name: "no VMs", want: ""},
		{name: "one backend's", vms: []string{"lima", "lima"}, want: "lima"},
		{name: "two backends', sorted", vms: []string{"vz", "fake", "lima"}, want: "fake lima vz"},
		{name: "unknown backends", vms: []string{"qemu", "vz"}, want: "vz"},
		{name: "a VM's policy file", vms: []string{"lima"}, files: "0123456789abc0/policy", want: "lima"},
		{name: "a folder that isn't a VM's", vms: []string{"lima"}, files: "other/vm.json", want: "lima"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			r := testRegistry(t, "lima", "vz", "fake")
			for i, b := range tc.vms {
				newState(t, d, "devvm-x-0123456789abc"+strconv.Itoa(i), b)
			}
			if tc.files != "" {
				if err := state.WriteFile(filepath.Join(d.VMs(), tc.files), []byte(`{"backend": "fake"}`)); err != nil {
					t.Fatal(err)
				}
			}
			bs, err := r.Used(d)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, b := range bs {
				got = append(got, b.Name())
			}
			if g := strings.Join(got, " "); g != tc.want {
				t.Errorf("got %q, want %q", g, tc.want)
			}
		})
	}
}

// readVM returns devvm-x's state: made on the backend recorded when it's
// set, else none.
func readVM(t *testing.T, d paths.Dirs, recorded string) *state.VM {
	t.Helper()
	if recorded != "" {
		newState(t, d, "devvm-x-0123456789abcd", recorded)
	}
	vm, err := state.ReadVM(d, "devvm-x-0123456789abcd")
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestForWithoutTheDefault(t *testing.T) {
	d := testDirs(t)
	r := testRegistry(t, "vz")
	if _, err := r.For(readVM(t, d, "")); err == nil {
		t.Error("For a new VM without lima: want an error")
	}
}

// A VM's state without its backend is never read: it isn't a VM's.
func TestForStateWithoutBackend(t *testing.T) {
	d := testDirs(t)
	err := state.WriteFile(d.VM("devvm-x-0123456789abcd").State(),
		[]byte(`{"version": 1, "id": "x", "name": "devvm-x-0123456789abcd", "image": "base", "dir": "/p"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.ReadVM(d, "devvm-x-0123456789abcd")
	var ce *state.CorruptError
	if !errors.As(err, &ce) || ce.Path != d.VM("devvm-x-0123456789abcd").State() {
		t.Errorf("got %v, want a CorruptError for the VM's state", err)
	}
}
