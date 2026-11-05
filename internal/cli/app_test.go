package cli

import (
	"errors"
	"testing"
)

// One run makes each service once, however many commands and services
// ask for it, and the services share it: the VM environment's registry
// and tools are the run's, and so are the builder's.
func TestAppMakesEachServiceOnce(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("DEVVM_LIMA_HOME", t.TempDir())
	u, _, _ := testUI()
	a := newApp(u)
	for range 3 {
		if _, err := a.builder(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.vmEnv(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.registry(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.tools(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.secrets(); err != nil {
			t.Fatal(err)
		}
		if _, err := a.dirs(); err != nil {
			t.Fatal(err)
		}
		a.host()
	}
	for name, n := range map[string]int{
		"dirs": a.folders.n, "overrides": a.overrides.n, "secrets": a.store.n, "registry": a.reg.n,
		"tools": a.tl.n, "vm env": a.vm.n, "builder": a.b.n,
	} {
		if n != 1 {
			t.Errorf("%s made %d times, want once", name, n)
		}
	}
	env, _ := a.vmEnv()
	reg, _ := a.registry()
	tl, _ := a.tools()
	b, _ := a.builder()
	if env.Registry != reg || env.Tools != tl || b.Tools != tl {
		t.Error("a service isn't the run's")
	}
}

// A service that fails to be made fails for the rest of the run, without
// being made again.
func TestAppKeepsAFailure(t *testing.T) {
	var m made[int]
	calls := 0
	fail := errors.New("no")
	for range 2 {
		if _, err := m.get(func() (int, error) { calls++; return 0, fail }); !errors.Is(err, fail) {
			t.Errorf("err = %v", err)
		}
	}
	if calls != 1 || m.n != 1 {
		t.Errorf("made %d times", calls)
	}
}
