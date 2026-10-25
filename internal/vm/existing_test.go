package vm

import (
	"errors"
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

// existingVM makes the folder's VM with up, and returns its state.
func existingVM(t *testing.T, e *Env, o Options) *state.VM {
	t.Helper()
	if err := e.Up(t.Context(), o); err != nil {
		t.Fatal(names(err))
	}
	vm := vmName(t, e)
	t.Cleanup(func() { e.Proxy.Stop(vm) }) // its proxy, if any: the test's sleep
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func vmName(t *testing.T, e *Env) string {
	t.Helper()
	es, err := os.ReadDir(e.Dirs.VMs())
	if err != nil || len(es) == 0 {
		t.Fatalf("no VM state: %v", err)
	}
	return es[0].Name()
}

// answers gives e's questions a terminal and these answers.
func answers(t *testing.T, e *Env, yes bool) *[]string {
	t.Helper()
	var asked []string
	e.Terminal = func() bool { return true }
	e.Confirm = func(m msg.M) bool { asked = append(asked, m.Names()); return yes }
	e.Ask = e.Confirm
	return &asked
}

// With network policy: a running VM isn't started again and keeps its
// proxy; the self-check runs again on every launch. A stopped one is
// started confined.
func TestOpenNetworkPolicy(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{Image: "base", NetworkPolicy: true})
	vm := st.Name()
	pid, ok := e.Proxy.PID(vm)
	if !ok {
		t.Fatal("no proxy")
	}
	before := len(f.Calls())
	if err := e.Up(t.Context(), Options{}); err != nil {
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
	if err := e.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); !slices.Contains(f.Methods()[before:], "Start") || c.Net != backend.NetProxy {
		t.Errorf("calls = %q, confinement %+v", f.Methods()[before:], c)
	}
	// Started outside devvm: no terminal to ask in, so it's stopped.
	f.Unconfined = true
	if err := e.Up(t.Context(), Options{}); names(err) != "net_unenforced_stop vm="+vm {
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

// devvm network: shown as devvm ls shows it; on and off change the state
// only, applied when the VM next starts. A running VM turned on isn't
// confined, so its next launch offers the restart; one turned off stays
// on its proxy until it starts again.
func TestNetwork(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	shown := func(want string) {
		t.Helper()
		var b strings.Builder
		if err := e.Network(t.Context(), &b); err != nil || b.String() != want+"\n" {
			t.Errorf("Network = %q, %v; want %s", b.String(), names(err), want)
		}
	}
	shown("off")
	for range 2 { // the second time it's already on
		if err := e.SetNetwork(t.Context(), true); err != nil {
			t.Fatal(names(err))
		}
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(KeyNet) != "on" || slices.Contains(f.Methods(), "Stop") {
		t.Errorf("net=%q, calls %q", st.Get(KeyNet), f.Methods())
	}
	shown(netpolicy.CheckUnenforced)
	// Its next launch, without a terminal to offer the restart in, stops
	// it; the one after starts it confined.
	if err := e.Up(t.Context(), Options{}); names(err) != "net_unenforced_stop vm="+vm {
		t.Fatalf("Up = %v", names(err))
	}
	if err := e.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetProxy {
		t.Errorf("started %+v", c)
	}
	shown("on")
	// Off while it runs confined: it keeps its proxy until it starts again.
	if err := e.SetNetwork(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	pid, ok := e.Proxy.PID(vm)
	before := len(f.Calls())
	if err := e.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if again, _ := e.Proxy.PID(vm); !ok || again != pid || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("proxy %d, was %d; calls %q", again, pid, f.Methods()[before:])
	}
	shown("off")
	if err := e.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetDirect {
		t.Errorf("restarted %+v", c)
	}
	if _, ok := e.Proxy.PID(vm); ok {
		t.Error("its proxy still runs")
	}
}

// A backend that can't confine a VM to its proxy refuses devvm network
// on, changing nothing.
func TestNetworkRequiresEgressProxy(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureEgressProxy] = backend.Emulated(false)
	e, _, _ := upEnv(t, caps)
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	err := e.SetNetwork(t.Context(), true)
	if got := names(err); got != "backend_unreviewed backend=fake feature=egress-proxy vm="+st.Name() {
		t.Errorf("Network on = %v", got)
	}
	if st, _ := state.ReadVM(e.Dirs, st.Name()); st.Get(KeyNet) != "off" {
		t.Errorf("net=%q", st.Get(KeyNet))
	}
}

// An image naming what the VM lacks adds it in place, after asking: the
// tools join its list, a layer's script runs in it, and new agents'
// folders are mounted, which stops it first. Declined, nothing changes.
func TestAddLayers(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
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
	if err := e.Up(t.Context(), Options{Image: "foo+claude"}); err != nil {
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
	if err := e.Up(t.Context(), Options{Image: "foo+claude"}); err != nil {
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
	if err := e.Up(t.Context(), Options{Image: "claude"}); err != nil || len(*asked) > 0 {
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

// rm exports an agent VM's history first; a failed export keeps the VM.
// One made for an earlier folder at this path is deleted only after
// asking, without an export.
func TestRm(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{Image: "claude", NoNetworkPolicy: true})
	vm := st.Name()
	f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		if len(argv) == 3 && strings.Contains(argv[2], "tar -czf") {
			return errors.New("tar failed")
		}
		return nil
	}
	if err := e.Rm(t.Context(), false); names(err) != "rm_export_failed vm="+vm {
		t.Errorf("Rm = %v", names(err))
	}
	if _, ok := f.Instances[vm]; !ok {
		t.Fatal("deleted after a failed export")
	}
	if es, _ := os.ReadDir(e.Dirs.Exports()); len(es) > 0 {
		t.Errorf("exports %v", es)
	}
	var script string
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		if len(argv) == 3 && strings.Contains(argv[2], "tar -czf") {
			script = argv[2]
			_, err := o.Stdout.Write([]byte("tarball"))
			return err
		}
		return nil
	}
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	if !strings.Contains(script, "ls -d .claude .claude.json .codex") || !strings.Contains(script, "--exclude=.claude/.credentials.json") {
		t.Errorf("export script %q", script)
	}
	es, _ := os.ReadDir(e.Dirs.Exports())
	if len(es) != 1 || !strings.HasPrefix(es[0].Name(), vm+"-") {
		t.Fatalf("exports %v", es)
	}
	if data, _ := os.ReadFile(filepath.Join(e.Dirs.Exports(), es[0].Name())); string(data) != "tarball" {
		t.Errorf("export = %q", data)
	}
	if _, ok := f.Instances[vm]; ok {
		t.Error("not deleted")
	}
	if _, err := os.Stat(e.Dirs.VMState(vm)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state: %v", err)
	}

	// Another folder at the same path.
	st = existingVM(t, e, Options{Image: "claude", NoNetworkPolicy: true})
	if err := st.Set(state.KeyFolder, "0:0"); err != nil {
		t.Fatal(err)
	}
	asked := answers(t, e, false)
	if err := e.Rm(t.Context(), true); err != nil {
		t.Fatal(names(err))
	}
	if _, ok := f.Instances[vm]; !ok || !slices.Equal(*asked, []string{"delete_other_folder_vm vm=" + vm + " was=" + e.Dir}) {
		t.Fatalf("declined: asked %q", *asked)
	}
	answers(t, e, true)
	script = ""
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	if _, ok := f.Instances[vm]; ok || script != "" {
		t.Errorf("confirmed: deleted %v, export %q", !ok, script)
	}
}

// export saves the history to the exports folder or a file of the user's,
// which a failed export leaves as it was; the VM is kept either way.
func TestExport(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	vm := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true}).Name()
	tarball := "tarball"
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		if len(argv) == 3 && strings.Contains(argv[2], "tar -czf") {
			if tarball == "" {
				_, _ = o.Stdout.Write([]byte("partial"))
				return errors.New("tar failed")
			}
			_, err := o.Stdout.Write([]byte(tarball))
			return err
		}
		return nil
	}
	if err := e.Export(t.Context(), ""); err != nil {
		t.Fatal(names(err))
	}
	es, _ := os.ReadDir(e.Dirs.Exports())
	if len(es) != 1 || !strings.HasPrefix(es[0].Name(), vm+"-") || !strings.HasSuffix(es[0].Name(), ".tar.gz") {
		t.Fatalf("exports %v", es)
	}

	dir := filepath.Join(t.TempDir(), "new")
	file := filepath.Join(dir, "h.tar.gz")
	read := func() string {
		data, _ := os.ReadFile(file) //nolint:gosec // the test's own folder
		return string(data)
	}
	if err := e.Export(t.Context(), file); err != nil {
		t.Fatal(names(err))
	}
	if data := read(); data != "tarball" {
		t.Errorf("export = %q", data)
	}
	tarball = ""
	err := e.Export(t.Context(), file)
	if le, ok := errors.AsType[*backend.LoggedError](err); !ok || names(le.Msg) != "history_not_exported vm="+vm {
		t.Errorf("failed: %v", names(err))
	}
	if data := read(); data != "tarball" {
		t.Errorf("after a failed export = %q", data)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("left %v", es)
	}
	if _, ok := f.Instances[vm]; !ok {
		t.Error("deleted")
	}
}

// adopt gives the folder the VM made for an earlier one at its path,
// after asking; it's then opened, its .git locked as new.
func TestAdopt(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	if err := e.Adopt(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if err := st.Set(state.KeyFolder, "0:0"); err != nil {
		t.Fatal(err)
	}
	if err := e.Up(t.Context(), Options{}); names(err) != "vm_other_folder vm="+st.Name()+" was="+e.Dir {
		t.Errorf("Up = %v", names(err))
	}
	asked := answers(t, e, false)
	if err := e.Adopt(t.Context()); err != nil || len(*asked) != 1 {
		t.Fatalf("declined: %v, asked %q", names(err), *asked)
	}
	answers(t, e, true)
	if err := e.Adopt(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if err := e.Up(t.Context(), Options{}); err != nil {
		t.Errorf("Up adopted = %v", names(err))
	}
}

// remove and update open the VM after changing its tools; restart stops
// it first.
func TestRemoveUpdateRestart(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{Image: "go+claude", NoNetworkPolicy: true})
	vm := st.Name()
	if err := e.Remove(t.Context(), Options{}, "claude"); err != nil {
		t.Fatal(names(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "go" || strings.Contains(st.Get(KeyTools), "claude") {
		t.Errorf("state image=%s tools=%s", st.Get(state.KeyImage), st.Get(KeyTools))
	}
	if err := e.Remove(t.Context(), Options{}, "python"); names(err) != "vm_lacks has=go layer=python vm="+vm {
		t.Errorf("Remove python = %v", names(err))
	}
	if err := e.Update(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	before := len(f.Calls())
	if err := e.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "Stop"), slices.Index(ms, "Start"); i < 0 || j < i {
		t.Errorf("restart calls = %q", ms)
	}
}
