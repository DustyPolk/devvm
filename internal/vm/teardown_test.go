package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// everyFile writes a file at each of a VM's paths in its folder, as its
// runs would leave them: its proxy's, its sandbox profile, its links',
// its secrets', its logs and its backend's.
func everyFile(t *testing.T, e *Env, vm string) {
	t.Helper()
	v := e.Dirs.VM(vm)
	for _, p := range []string{
		v.Policy(), v.Proxy(".pid"), v.Proxy(".auth"), v.Profile(), v.Started(), v.Secrets(), v.SecretsPolicy(),
		v.LinksPID(), v.LinkList(), v.EventLog(), v.EventLog() + ".1", v.ProxyLog(), v.LinksLog(),
		v.Ports(), v.Forwarder(".log"), v.Forwarder(".lock"),
		filepath.Join(v.Run(), ".proxy.auth.tmp-1"), // a write that never finished
	} {
		if err := state.WriteFile(p, []byte("x\n")); err != nil {
			t.Fatal(err)
		}
	}
}

// vmsLeft lists what's left in the VMs' folder.
func vmsLeft(t *testing.T, e *Env) []string {
	t.Helper()
	var left []string
	err := filepath.WalkDir(e.Dirs.VMs(), func(p string, _ os.DirEntry, err error) error {
		if err != nil || p == e.Dirs.VMs() {
			return err
		}
		left = append(left, p)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return left
}

// outsideFolder lists the files and folders in devvm's data folder that
// are named by the VM, by its name or its hash, outside its own folder:
// what a path built outside internal/paths would leave, which its
// teardown would miss.
func outsideFolder(t *testing.T, e *Env, vm string) []string {
	t.Helper()
	h := vm[len(vm)-14:]
	var out []string
	err := filepath.WalkDir(e.Dirs.Data, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == e.Dirs.VM(vm).Dir {
			return filepath.SkipDir
		}
		if b := filepath.Base(p); strings.Contains(b, vm) || strings.Contains(b, h) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// devvm rm's teardown deletes every file the VM had, in its folder and
// out of it; so does devvm gc --orphans's, for a VM its backend lost.
// A VM with network policy, its proxy running, has no file named for it
// outside its folder.
func TestTeardownLeavesNothing(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{NetworkPolicy: true})
	if _, ok := e.Proxy.PID(st.Name()); !ok {
		t.Fatal("no proxy runs")
	}
	if out := outsideFolder(t, e, st.Name()); len(out) > 0 {
		t.Errorf("files of %s outside its folder: %q", st.Name(), out)
	}
	everyFile(t, e, st.Name())
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true, NoExport: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if left := vmsLeft(t, e); len(left) > 0 {
		t.Errorf("rm left %q", left)
	}
	if _, ok := e.Proxy.PID(st.Name()); ok {
		t.Error("its proxy still runs")
	}

	// An orphan: its backend lost it, and its state is over an hour old.
	st = existingVM(t, e, Options{NoNetworkPolicy: true})
	everyFile(t, e, st.Name())
	delete(f.Instances, st.Name())
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(e.Dirs.VM(st.Name()).State(), old, old); err != nil {
		t.Fatal(err)
	}
	r, err := (Leftovers{e}).Find(t.Context())
	if err != nil || len(r.Orphans) != 1 || r.Orphans[0].Name != st.Name() {
		t.Fatalf("Find = %+v, %v", r, err)
	}
	if err := (Leftovers{e}).Forget(t.Context(), r.Orphans[0]); err != nil {
		t.Fatal(err)
	}
	if left := vmsLeft(t, e); len(left) > 0 {
		t.Errorf("gc --orphans left %q", left)
	}
}

// A new VM made where an earlier one's folder was left (its backend lost
// it) gets a clean folder: none of the earlier VM's files.
func TestCreateClearsFolder(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	everyFile(t, e, st.Name())
	delete(f.Instances, st.Name())
	again := existingVM(t, e, Options{NoNetworkPolicy: true})
	if again.Name() != st.Name() || again.Get(state.KeyID) == st.Get(state.KeyID) {
		t.Errorf("made again: %s (id %s), was %s (id %s)", again.Name(), again.Get(state.KeyID), st.Name(), st.Get(state.KeyID))
	}
	for _, p := range []string{e.Dirs.VM(st.Name()).EventLog(), e.Dirs.VM(st.Name()).Policy(), e.Dirs.VM(st.Name()).Ports()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s left: %v", p, err)
		}
	}
}

// A folder's VM is found by its path's hash, under the name and on the
// backend it was made with: made on a backend whose names leave all of a
// long folder's name, a later devvm whose new VMs get another backend
// (whose names would cut it) finds that VM, and makes no second one.
func TestLookupOtherBackend(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.NameLimit = 32
	long := filepath.Join(filepath.Dir(e.Dir), "a-very-long-project-folder")
	if err := os.MkdirAll(long, 0o700); err != nil {
		t.Fatal(err)
	}
	e.Dir = long
	other := backendtest.New("other", allNative())
	other.ShellCmd = f.ShellCmd
	reg, err := backend.NewRegistry(f, other)
	if err != nil {
		t.Fatal(err)
	}
	reg.NewVMs = "other"
	e.Registry = reg
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	if st.Get(state.KeyBackend) != "other" || len(st.Name()) <= f.NameLimit {
		t.Fatalf("made %s on %s", st.Name(), st.Get(state.KeyBackend))
	}
	reg.NewVMs = "fake"
	own, _, err := e.lookup(t.Context(), true)
	if err != nil || own.f.Name != st.Name() || own.b.Name() != "other" || !own.exists {
		t.Errorf("lookup = %+v, %v", own, err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if names := vmNames(t, e); !slices.Equal(names, []string{st.Name()}) || slices.Contains(f.Methods(), "CreateVM") {
		t.Errorf("VMs %q, the default backend's calls %q", names, f.Methods())
	}
}
