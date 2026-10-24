package backend_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
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
	r := testRegistry(t, "smolvm", "lima")
	if got := r.Names(); len(got) != 2 || got[0] != "lima" || got[1] != "smolvm" {
		t.Errorf("Names: %q", got)
	}
	if b, ok := r.Lookup("smolvm"); !ok || b.Name() != "smolvm" {
		t.Errorf("Lookup(smolvm): %v, %v", b, ok)
	}
	if _, ok := r.Lookup("docker"); ok {
		t.Error("Lookup(docker): found")
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
		{name: "a new VM, the default file", file: "smolvm", want: "smolvm"},
		{name: "a new VM, --backend", file: "smolvm", flag: "docker", want: "docker"},
		{name: "an existing VM keeps its backend", file: "docker", recorded: "smolvm", want: "smolvm"},
		{name: "an existing VM ignores --backend", recorded: "smolvm", flag: "docker", want: "smolvm", ignored: true},
		{name: "--backend naming its own", recorded: "smolvm", flag: "smolvm", want: "smolvm"},
		{name: "an unknown --backend", flag: "sbx", want: "backend_unknown backends=docker, lima, smolvm name=sbx"},
		{name: "an unknown default", file: "sbx", want: "backend_unknown_default backends=docker, lima, smolvm file=$CONFIG/backend name=sbx"},
		{name: "an existing VM on a backend not here", recorded: "qemu", want: "backend_unknown_vm backend=qemu vm=devvm-x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDirs(t)
			r := testRegistry(t, "lima", "smolvm", "docker")
			if tc.file != "" {
				if err := state.WriteBackend(d, tc.file); err != nil {
					t.Fatal(err)
				}
			}
			vm := readVM(t, d, tc.recorded)
			c, err := r.For(d, vm, tc.flag)
			if err != nil {
				want := strings.ReplaceAll(tc.want, "$CONFIG", d.Config)
				if got := names(t, err); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
				return
			}
			if c.Backend.Name() != tc.want || c.FlagIgnored != tc.ignored {
				t.Errorf("got %s (flag ignored %v), want %s (%v)", c.Backend.Name(), c.FlagIgnored, tc.want, tc.ignored)
			}
		})
	}
}

// readVM returns devvm-x's state: made with backend= when recorded is set,
// else none.
func readVM(t *testing.T, d paths.Dirs, recorded string) *state.VM {
	t.Helper()
	if recorded != "" {
		if _, err := state.CreateVM(d, "devvm-x", state.NewVM{Image: "base", Backend: recorded}); err != nil {
			t.Fatal(err)
		}
	}
	vm, err := state.ReadVM(d, "devvm-x")
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestForWithoutTheDefault(t *testing.T) {
	d := testDirs(t)
	r := testRegistry(t, "smolvm")
	_, err := r.For(d, readVM(t, d, ""), "")
	if got, want := names(t, err), "backend_unknown backends=smolvm name=lima"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestForStateWithoutBackend(t *testing.T) {
	d := testDirs(t)
	if _, err := state.CreateVM(d, "devvm-x", state.NewVM{Image: "base"}); err != nil {
		t.Fatal(err)
	}
	_, err := testRegistry(t, "lima").For(d, readVM(t, d, ""), "")
	var ce *state.CorruptError
	if !errors.As(err, &ce) || ce.Path != d.VMState("devvm-x") {
		t.Errorf("got %v, want a CorruptError for the VM's state", err)
	}
}

func TestForBadDefaultFile(t *testing.T) {
	d := testDirs(t)
	if err := os.MkdirAll(d.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.Backend(), []byte("lima\nsmolvm\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := testRegistry(t, "lima").For(d, readVM(t, d, ""), "")
	var ce *state.CorruptError
	if !errors.As(err, &ce) {
		t.Errorf("got %v, want a CorruptError", err)
	}
}
