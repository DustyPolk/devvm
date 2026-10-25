package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// With network policy: a running VM isn't started again and keeps its
// proxy; the self-check runs again on every launch. A stopped one is
// started confined.
func TestOpenNetworkPolicy(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	l := Launch{e}
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{Image: "base", NetworkPolicy: true})
	vm := st.Name()
	pid, ok := e.Proxy.PID(vm)
	if !ok {
		t.Fatal("no proxy")
	}
	before := len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	ms := f.Methods()[before:]
	if slices.Contains(ms, "Start") || !slices.Contains(ms, "Confined") {
		t.Errorf("calls = %q", ms)
	}
	if again, _ := e.Proxy.PID(vm); again != pid {
		t.Errorf("proxy %d, was %d", again, pid)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); !slices.Contains(f.Methods()[before:], "Start") || c.Net != backend.NetProxy {
		t.Errorf("calls = %q, confinement %+v", f.Methods()[before:], c)
	}
	// Started outside devvm: no terminal to ask in, so it's stopped.
	f.Unconfined = true
	if err := l.Up(t.Context(), Options{}); names(err) != "net_unenforced_stop vm="+vm {
		t.Errorf("Up unconfined = %v", names(err))
	}
	if f.Instances[vm].Status != backend.StatusStopped || len(*execs) != 3 {
		t.Errorf("status %s, execs %d", f.Instances[vm].Status, len(*execs))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(netpolicy.KeyCheck) != netpolicy.CheckUnenforced {
		t.Errorf("net_check = %q", st.Get(netpolicy.KeyCheck))
	}
}

// An image naming what the VM lacks adds it in place, after asking: the
// tools join its list, a layer's script runs in it, and new agents'
// folders are mounted, which stops it first. Declined, nothing changes.
func TestAddLayers(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	if err := os.MkdirAll(e.Dirs.Layers(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Dirs.Layers(), "foo.sh"), []byte("#!/bin/bash\necho foo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skills := filepath.Join(e.Home, ".claude", "skills")
	if err := os.MkdirAll(skills, 0o700); err != nil {
		t.Fatal(err)
	}
	asked := answers(t, e, false)
	before := len(f.Calls())
	if err := l.Up(t.Context(), Options{Image: "foo+claude"}); err != nil {
		t.Fatal(names(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if !slices.Equal(*asked, []string{"add_in_place image=base missing=foo claude vm=" + vm}) || st.Get(state.KeyImage) != "base" ||
		slices.Contains(f.Methods()[before:], "BuildTools") {
		t.Errorf("declined: asked %q, image %s, calls %q", *asked, st.Get(state.KeyImage), f.Methods()[before:])
	}

	answers(t, e, true)
	var scripts []string
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		if slices.Equal(argv, []string{"bash", "-s"}) {
			b, _ := readAll(o)
			scripts = append(scripts, b)
		}
		return nil
	}
	before = len(f.Calls())
	if err := l.Up(t.Context(), Options{Image: "foo+claude"}); err != nil {
		t.Fatal(names(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "foo+claude" || !strings.HasPrefix(st.Get(KeyTools), "claude-") {
		t.Errorf("state image=%s tools=%s", st.Get(state.KeyImage), st.Get(KeyTools))
	}
	if len(scripts) != 1 || !strings.Contains(scripts[0], "echo foo") {
		t.Errorf("scripts %q", scripts)
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "Stop"), slices.Index(ms, "SetAgentDirs"); i < 0 || j < i || !slices.Contains(ms, "BuildTools") {
		t.Errorf("calls = %q", ms)
	}
	if dirs := f.Instances[vm].AgentDirs; !slices.Equal(dirs, []string{skills}) {
		t.Errorf("agent dirs = %q", dirs)
	}
	// It has them now: nothing to add or ask.
	*asked = nil
	if err := l.Up(t.Context(), Options{Image: "claude"}); err != nil || len(*asked) > 0 {
		t.Errorf("again: %v, asked %q", names(err), *asked)
	}
}

func readAll(o backend.ExecOpts) (string, error) {
	var b strings.Builder
	buf := make([]byte, 512)
	for {
		n, err := o.Stdin.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String(), nil //nolint:nilerr // EOF
		}
	}
}

// remove and update open the VM after changing its tools; restart stops
// it first.
func TestRemoveUpdateRestart(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	st := existingVM(t, e, Options{Image: "go+claude", NoNetworkPolicy: true})
	vm := st.Name()
	if err := l.Remove(t.Context(), Options{}, "claude"); err != nil {
		t.Fatal(names(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "go" || strings.Contains(st.Get(KeyTools), "claude") {
		t.Errorf("state image=%s tools=%s", st.Get(state.KeyImage), st.Get(KeyTools))
	}
	if err := l.Remove(t.Context(), Options{}, "python"); names(err) != "vm_lacks has=go layer=python vm="+vm {
		t.Errorf("Remove python = %v", names(err))
	}
	if err := l.Update(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	before := len(f.Calls())
	if err := l.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "Stop"), slices.Index(ms, "Start"); i < 0 || j < i {
		t.Errorf("restart calls = %q", ms)
	}
}
