package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
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
		t.Fatal(msg.NamesOf(err))
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
		t.Fatal(msg.NamesOf(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); !slices.Contains(f.Methods()[before:], "Start") || c.Net != backend.NetProxy {
		t.Errorf("calls = %q, confinement %+v", f.Methods()[before:], c)
	}
	// Started outside devvm: no terminal to ask in, so it's stopped.
	f.Unconfined = true
	if err := l.Up(t.Context(), Options{}); msg.NamesOf(err) != "net_unenforced_stop vm="+vm {
		t.Errorf("Up unconfined = %v", msg.NamesOf(err))
	}
	if f.Instances[vm].Status != backend.StatusStopped || len(*execs) != 3 {
		t.Errorf("status %s, execs %d", f.Instances[vm].Status, len(*execs))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(netpolicy.KeyCheck) != netpolicy.CheckUnenforced {
		t.Errorf("net_check = %q", st.Get(netpolicy.KeyCheck))
	}
}

// An image naming what the VM lacks is refused, and nothing changes: a
// layer it lacks first, which only a new VM gets, else the shared tools
// it lacks, which devvm tools add adds. Naming what it has opens it.
func TestImageLacks(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	st := existingVM(t, e, Options{Image: "go", NoNetworkPolicy: true})
	vm := st.Name()
	if err := os.MkdirAll(e.Dirs.Layers(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Dirs.Layers(), "foo.sh"), []byte("#!/bin/bash\necho foo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := len(f.Calls())
	for _, tc := range []struct{ image, want string }{
		{"foo+claude", "vm_lacks_layer image=go+foo+claude layer=foo vm=" + vm},
		{"docker", "vm_lacks_layer image=go+docker layer=docker vm=" + vm},
		{"python+claude", "vm_lacks_tools tools=python claude vm=" + vm},
		{"claude", "vm_lacks_tools tools=claude vm=" + vm},
	} {
		if err := l.Up(t.Context(), Options{Image: tc.image}); msg.NamesOf(err) != tc.want {
			t.Errorf("Up %s = %v, want %s", tc.image, msg.NamesOf(err), tc.want)
		}
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if ms := f.Methods()[before:]; st.Get(state.KeyImage) != "go" || slices.Contains(ms, "Stop") || slices.Contains(ms, "BuildTools") ||
		slices.Contains(ms, "Exec") {
		t.Errorf("image %s, calls %q", st.Get(state.KeyImage), ms)
	}
	for _, img := range []string{"go", "base"} {
		if err := l.Up(t.Context(), Options{Image: img}); err != nil {
			t.Errorf("Up %s = %v", img, msg.NamesOf(err))
		}
	}
}

// devvm tools add: the tools the share lacks built, then in the VM's
// image name and list; a new agent's config folders mounted at once on a
// stopped VM, after asking on a running one; those it has said and left.
func TestToolsAdd(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	tl := Tools{e}
	st := existingVM(t, e, Options{Image: "go", NoNetworkPolicy: true})
	vm := st.Name()
	skills := filepath.Join(e.Home, ".claude", "skills")
	if err := os.MkdirAll(skills, 0o700); err != nil {
		t.Fatal(err)
	}
	var said msg.Recorder
	e.Info = said.Add
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before := len(f.Calls())
	if err := tl.Add(t.Context(), Options{}, []string{"go", "claude", "claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "go+claude" || !strings.Contains(st.Get(KeyTools), " claude-") {
		t.Errorf("state image=%s tools=%s", st.Get(state.KeyImage), st.Get(KeyTools))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "SetAgentDirs"), slices.Index(ms, "Start"); !slices.Contains(ms, "BuildTools") || i < 0 || j < i {
		t.Errorf("calls = %q", ms)
	}
	if dirs := f.Instances[vm].AgentDirs; !slices.Equal(dirs, []string{skills}) {
		t.Errorf("agent dirs = %q", dirs)
	}
	if !slices.Contains(said.Names(), "has_tools tools=go vm="+vm) || !slices.Contains(said.Names(), "vm_is_now image=go+claude vm="+vm) {
		t.Errorf("said %q", said.Names())
	}

	// Running: a new agent's folders after a restart it asks for.
	prompts := filepath.Join(e.Home, ".codex", "prompts")
	if err := os.MkdirAll(prompts, 0o700); err != nil {
		t.Fatal(err)
	}
	asked := answers(t, e, true)
	before = len(f.Calls())
	if err := tl.Add(t.Context(), Options{}, []string{"codex"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); st.Get(state.KeyImage) != "go+claude+codex" {
		t.Errorf("image = %s", st.Get(state.KeyImage))
	}
	ms = f.Methods()[before:]
	if !slices.Equal(*asked, []string{"update_mounts vm=" + vm}) || !slices.Contains(ms, "Stop") ||
		!slices.Contains(f.Instances[vm].AgentDirs, prompts) {
		t.Errorf("asked %q, calls %q, agent dirs %q", *asked, ms, f.Instances[vm].AgentDirs)
	}
}

// tools rm and update open the VM after changing its tools; restart stops
// it first.
func TestRemoveUpdateRestart(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	tl := Tools{e}
	st := existingVM(t, e, Options{Image: "go+claude", NoNetworkPolicy: true})
	vm := st.Name()
	if err := tl.Remove(t.Context(), Options{}, []string{"claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "go" || strings.Contains(st.Get(KeyTools), "claude") {
		t.Errorf("state image=%s tools=%s", st.Get(state.KeyImage), st.Get(KeyTools))
	}
	if err := tl.Remove(t.Context(), Options{}, []string{"python"}); msg.NamesOf(err) != "vm_lacks has=go layer=python vm="+vm {
		t.Errorf("Remove python = %v", msg.NamesOf(err))
	}
	if err := tl.Update(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if ts, err := tl.List(t.Context()); err != nil || len(ts) != 1 || ts[0].Tool != "go" || ts[0].Newer != "" {
		t.Errorf("List = %+v, %v", ts, err)
	}
	// claude added after go, listed first by the image: still the newest.
	if err := tl.Add(t.Context(), Options{}, []string{"claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := tl.Remove(t.Context(), Options{}, []string{"go"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := tl.Add(t.Context(), Options{}, []string{"go"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	had := st.Get(KeyTools)
	if err := tl.Update(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); !strings.HasPrefix(had, "claude-") || st.Get(KeyTools) != had {
		t.Errorf("Update after adding: tools %q, were %q", st.Get(KeyTools), had)
	}
	before := len(f.Calls())
	if err := l.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "Stop"), slices.Index(ms, "Start"); i < 0 || j < i {
		t.Errorf("restart calls = %q", ms)
	}
}
