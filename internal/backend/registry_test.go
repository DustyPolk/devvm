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
		file     string // the default backend file; "" for none
		recorded string // the VM's backend=; "" for a new VM
		flag     string
		want     string // the backend chosen, or the error's names
		ignored  bool
	}{
		{name: "a new VM, nothing set", want: "lima"},
		{name: "a new VM, the default file", file: "vz", want: "vz"},
		{name: "a new VM, --backend", file: "vz", flag: "fake", want: "fake"},
		{name: "an existing VM keeps its backend", file: "fake", recorded: "vz", want: "vz"},
		{name: "an existing VM ignores --backend", recorded: "vz", flag: "fake", want: "vz", ignored: true},
		{name: "--backend naming its own", recorded: "vz", flag: "vz", want: "vz"},
		{name: "an unknown --backend", flag: "sbx", want: "backend_unknown backends=fake, lima, vz name=sbx"},
		{name: "an unknown default", file: "sbx", want: "backend_unknown_default backends=fake, lima, vz file=/c/config.toml name=sbx"},
		{name: "an existing VM on a backend not here", recorded: "qemu", want: "backend_unknown_vm backend=qemu vm=devvm-x-0123456789abcd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			r := testRegistry(t, "lima", "vz", "fake")
			if tc.file != "" {
				r.DefaultBackend = func() (string, string, error) { return tc.file, "/c/config.toml", nil }
			}
			vm := readVM(t, d, tc.recorded)
			c, err := r.For(d, vm, tc.flag)
			if err != nil {
				if got := msg.NamesOf(err); got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
				return
			}
			if c.Backend.Name() != tc.want || c.FlagIgnored != tc.ignored {
				t.Errorf("got %s (flag ignored %v), want %s (%v)", c.Backend.Name(), c.FlagIgnored, tc.want, tc.ignored)
			}
		})
	}
}

// Used is the registered backends some VM's state names, sorted: a state
// without backend= and one naming a backend not here count for none.
func TestUsed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vms   []string // each VM's backend=, "" for none
		want  string
		files string // a file that isn't a VM's state, in their folder
	}{
		{name: "no VMs", want: ""},
		{name: "one backend's", vms: []string{"lima", "lima"}, want: "lima"},
		{name: "two backends', sorted", vms: []string{"vz", "fake", "lima"}, want: "fake lima vz"},
		{name: "unrecorded and unknown backends", vms: []string{"", "qemu", "vz"}, want: "vz"},
		{name: "a VM's policy file", vms: []string{"lima"}, files: "0123456789abc0/policy", want: "lima"},
		{name: "a folder that isn't a VM's", vms: []string{"lima"}, files: "other/state", want: "lima"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			r := testRegistry(t, "lima", "vz", "fake")
			for i, b := range tc.vms {
				if _, err := state.CreateVM(d, "devvm-x-0123456789abc"+strconv.Itoa(i), state.NewVM{Image: "base", Backend: b}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.files != "" {
				if err := state.WriteFile(filepath.Join(d.VMs(), tc.files), []byte("backend=fake\n")); err != nil {
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

// readVM returns devvm-x's state: made with backend= when recorded is set,
// else none.
func readVM(t *testing.T, d paths.Dirs, recorded string) *state.VM {
	t.Helper()
	if recorded != "" {
		if _, err := state.CreateVM(d, "devvm-x-0123456789abcd", state.NewVM{Image: "base", Backend: recorded}); err != nil {
			t.Fatal(err)
		}
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
	_, err := r.For(d, readVM(t, d, ""), "")
	if got, want := msg.NamesOf(err), "backend_unknown backends=vz name=lima"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestForStateWithoutBackend(t *testing.T) {
	d := testDirs(t)
	if _, err := state.CreateVM(d, "devvm-x-0123456789abcd", state.NewVM{Image: "base"}); err != nil {
		t.Fatal(err)
	}
	_, err := testRegistry(t, "lima").For(d, readVM(t, d, ""), "")
	var ce *state.CorruptError
	if !errors.As(err, &ce) || ce.Path != d.VM("devvm-x-0123456789abcd").State() {
		t.Errorf("got %v, want a CorruptError for the VM's state", err)
	}
}

// A new VM's default that can't be read fails, as it was read.
func TestForBadDefault(t *testing.T) {
	d := testDirs(t)
	r := testRegistry(t, "lima")
	bad := &state.CorruptError{Path: "/c/config.toml", Line: 1, Reason: "x"}
	r.DefaultBackend = func() (string, string, error) { return "", "", bad }
	if _, err := r.For(d, readVM(t, d, ""), ""); !errors.Is(err, bad) {
		t.Errorf("got %v, want %v", err, bad)
	}
}
