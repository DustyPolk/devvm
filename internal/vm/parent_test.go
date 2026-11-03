package vm

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
)

// subfolder makes dir (relative to the project folder web) and runs e in
// it.
func subfolder(t *testing.T, e *Env, web, dir string) string {
	t.Helper()
	sub := filepath.Join(web, dir)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	e.Dir = sub
	return sub
}

// lookupVM is the VM lookup finds for the folder e runs in, a parent's
// (its folder and the subfolder it's for) or the folder's own.
func lookupVM(t *testing.T, e *Env) (dir, sub string) {
	t.Helper()
	own, parent, err := e.lookup(t.Context(), true)
	if err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if parent == nil {
		if own.f.Dir != e.Dir || own.sub != "" {
			t.Errorf("own VM = %+v", own.f)
		}
		return "", ""
	}
	return parent.f.Dir, parent.sub
}

// A folder without a VM finds the nearest parent folder's that was made
// for it, stopping before the home folder.
func TestLookupParents(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{NoNetworkPolicy: true})
	lib := subfolder(t, e, web, "src/lib")
	if dir, sub := lookupVM(t, e); dir != web || sub != lib {
		t.Errorf("from src/lib: %q for %q, want web's", dir, sub)
	}
	// The nearest one wins.
	subfolder(t, e, web, "src")
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Here: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	src := e.Dir
	e.Dir = lib
	if dir, _ := lookupVM(t, e); dir != src {
		t.Errorf("from src/lib: %q, want src's", dir)
	}
	// Its own VM wins over both.
	e.Dir = src
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("from src: %q, want its own", dir)
	}
	// So does its state, when its backend has lost the VM.
	own, _, err := e.lookup(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	in := f.Instances[own.f.Name]
	delete(f.Instances, own.f.Name)
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("from src with its state alone: %q, want its own", dir)
	}
	// The nearest parent with state stops the search when its backend has
	// lost its VM: no VM, never web's.
	e.Dir = lib
	delete(f.Instances, own.f.Name)
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("past a parent's lost VM: %q, want none", dir)
	}
	f.Instances[own.f.Name] = in
	// And when its VM was made for an earlier folder at its path (a VM can
	// replace a nested folder through its mount): no VM, never web's.
	for _, n := range vmNames(t, e) {
		st, err := state.ReadVM(e.Dirs, n)
		if err != nil {
			t.Fatal(err)
		}
		if st.Get(state.KeyDir) == src {
			if err := st.Set(state.KeyFolder, "1:2"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("past another folder's VM: %q, want none", dir)
	}
	if _, err := e.findExisting(t.Context()); msg.NamesOf(err) != "no_vm dir="+lib {
		t.Errorf("findExisting past another folder's VM = %v, want no_vm", msg.NamesOf(err))
	}
	// Never the home folder's, or one above it.
	e.Dirs.Home = web
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("with web as home: %q", dir)
	}
	// Without parents, only its own.
	e.Dirs.Home = filepath.Dir(web)
	if _, p, err := e.lookup(t.Context(), false); err != nil || p != nil {
		t.Errorf("without parents: %+v, %v", p, err)
	}
}

func vmNames(t *testing.T, e *Env) []string {
	t.Helper()
	ns, err := state.VMNames(e.Dirs)
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

// up in a subfolder opens the parent's VM there: without a terminal, said
// in an info line; in one, after asking, a no making a new VM; with
// --here, a new VM without asking.
func TestUpInSubfolder(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{NoNetworkPolicy: true})
	var infos msg.Recorder
	e.Info = infos.Add
	src := subfolder(t, e, web, "src")
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	vm := vmName(t, e)
	if c := call(f, "Shell"); c.Args[0] != vm || c.Args[1] != src {
		t.Errorf("Shell = %v, want %s in %s", c.Args, vm, src)
	}
	if !slices.Contains(infos.Names(), "using_parent_vm dir="+web+" vm="+vm) {
		t.Errorf("infos = %q", infos.Names())
	}
	// Asked: yes opens it, no makes a new VM.
	asked := answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if want := "use_parent_vm dir=" + src + " parent=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if n := len(vmNames(t, e)); n != 1 {
		t.Errorf("%d VMs after yes", n)
	}
	answers(t, e, false)
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if n := len(vmNames(t, e)); n != 2 {
		t.Errorf("%d VMs after no", n)
	}
	// --clone: as --here, a new VM's checks (this subfolder isn't a
	// repo), nothing asked.
	subfolder(t, e, web, "tests")
	asked = answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Clone: true}); err == nil ||
		strings.HasPrefix(msg.NamesOf(err), "clone_needs_new_vm") || len(*asked) != 0 {
		t.Errorf("--clone: %v, asked %q", msg.NamesOf(err), *asked)
	}
	// --here: a new VM, nothing asked.
	subfolder(t, e, web, "docs")
	asked = answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Here: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if len(*asked) != 0 || len(vmNames(t, e)) != 3 {
		t.Errorf("--here: asked %q, %d VMs", *asked, len(vmNames(t, e)))
	}
}

// A parent's VM opens in the subfolder; a --clone VM's, in it when its
// clone has it, else at the clone's top.
func TestShellDir(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{NoNetworkPolicy: true})
	src := subfolder(t, e, web, "src")
	_, p, err := e.lookup(t.Context(), true)
	if err != nil || p == nil {
		t.Fatalf("lookup = %+v, %v", p, msg.NamesOf(err))
	}
	if got := p.shellDir(t.Context(), io.Discard); got != src {
		t.Errorf("shellDir = %q, want %q", got, src)
	}
	if err := p.st.Set(KeyMode, ModeClone); err != nil {
		t.Fatal(err)
	}
	var missing bool
	f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		if !slices.Equal(argv, []string{"test", "-d", src}) {
			t.Errorf("Exec %q", argv)
		}
		if missing {
			return errors.New("exit status 1")
		}
		return nil
	}
	if got := p.shellDir(t.Context(), io.Discard); got != src {
		t.Errorf("clone's shellDir = %q, want %q", got, src)
	}
	missing = true
	if got := p.shellDir(t.Context(), io.Discard); got != web {
		t.Errorf("clone without it: shellDir = %q, want %q", got, web)
	}
	p.sub = ""
	if got := p.shellDir(t.Context(), io.Discard); got != web {
		t.Errorf("its own: shellDir = %q, want %q", got, web)
	}
}

// Commands on the folder's VM use a parent's, saying so first; adopt
// doesn't.
func TestCommandsInSubfolder(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := vmName(t, e)
	var infos msg.Recorder
	e.Info = infos.Add
	subfolder(t, e, web, "src")
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if want := []string{"using_parent_vm dir=" + web + " vm=" + vm, "stopped vm=" + vm}; !slices.Equal(infos.Names(), want) {
		t.Errorf("infos = %q, want %q", infos.Names(), want)
	}
	if c := call(f, "Stop"); c.Args[0] != vm {
		t.Errorf("Stop = %v", c.Args)
	}
	if err := (Lifecycle{e}).Adopt(t.Context(), false); msg.NamesOf(err) != "no_vm dir="+e.Dir {
		t.Errorf("Adopt = %v", msg.NamesOf(err))
	}
	// rm: refused without a terminal, -f or not; asked in one, a no
	// keeping it.
	infos.Reset()
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true, NoExport: true}); msg.NamesOf(err) != "rm_parent_vm dir="+web+" vm="+vm {
		t.Errorf("Rm without a terminal = %v", msg.NamesOf(err))
	}
	if i := infos.Names(); len(i) == 0 || i[0] != "using_parent_vm dir="+web+" vm="+vm {
		t.Errorf("Rm's infos = %q", i)
	}
	asked := answers(t, e, false)
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true, NoExport: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if n := len(vmNames(t, e)); n != 1 {
		t.Errorf("%d VMs after no", n)
	}
	if want := "delete_parent_vm dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	asked = answers(t, e, true)
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true, NoExport: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if want := "delete_parent_vm dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if n := len(vmNames(t, e)); n != 0 {
		t.Errorf("%d VMs after rm", n)
	}
}

// Commands that give a parent folder's VM more access, run from a
// subfolder: refused without a terminal, asked in one ([y/N]), a no
// changing nothing. The same commands in the parent folder, or ones that
// take access away, ask nothing.
func TestWidenParentVM(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.ExecFunc = noDirect
	web := e.Dir
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	if err := (Net{e}).SetNetwork(t.Context(), true); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := st.Set(gitprotect.KeyGitProtect, gitprotect.On); err != nil {
		t.Fatal(err)
	}
	subfolder(t, e, web, "src")
	n, s := Net{e}, Settings{e}
	cases := []struct {
		cmd     string
		do      func() error
		changed func() bool
	}{
		{
			"devvm network off", func() error { return n.SetNetwork(t.Context(), false) },
			func() bool { return reread(t, e, vm).Get(KeyNet) == "off" },
		},
		{
			"devvm secret use", func() error { return n.SecretUse(t.Context(), "github", true) },
			func() bool { return slices.Contains(secrets.Names(reread(t, e, vm)), "github") },
		},
		{
			"devvm ssh-agent on", func() error { return s.SetSSHAgent(t.Context(), sshagent.On) },
			func() bool { return sshagent.Mode(reread(t, e, vm)) == sshagent.On },
		},
		{
			"devvm network allow", func() error { return n.PolicyChange(t.Context(), []string{"example.com"}, false, false, false) },
			func() bool { return policyHas(t, e, vm, "example.com") },
		},
		{
			"devvm git-protect off", func() error { return s.SetGitProtect(t.Context(), gitprotect.Off) },
			func() bool { return reread(t, e, vm).Get(gitprotect.KeyGitProtect) == gitprotect.Off },
		},
	}
	for _, c := range cases {
		e.Terminal = func() bool { return false }
		want := "widen_parent_vm_refused cmd=" + c.cmd + " dir=" + web + " vm=" + vm
		if err := c.do(); msg.NamesOf(err) != want {
			t.Errorf("%s without a terminal = %v, want %s", c.cmd, msg.NamesOf(err), want)
		}
		asked := answers(t, e, false)
		if err := c.do(); err != nil {
			t.Errorf("%s, answered no: %v", c.cmd, msg.NamesOf(err))
		}
		if want := "widen_parent_vm cmd=" + c.cmd + " dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
			t.Errorf("%s asked %q, want %q", c.cmd, *asked, want)
		}
		if c.changed() {
			t.Errorf("%s changed the VM", c.cmd)
		}
		if c.cmd == "devvm git-protect off" {
			continue // a running VM's locks don't change on macOS
		}
		answers(t, e, true)
		if err := c.do(); err != nil {
			t.Errorf("%s, answered yes: %v", c.cmd, msg.NamesOf(err))
		}
		if !c.changed() {
			t.Errorf("%s, answered yes, didn't change the VM", c.cmd)
		}
	}
	// Taking access away asks nothing, and so does network rm of an allow;
	// rm of a deny asks.
	asked := answers(t, e, false)
	if err := n.SetNetwork(t.Context(), true); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := n.PolicyChange(t.Context(), []string{"!bad.example.com"}, true, false, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := n.PolicyChange(t.Context(), []string{"example.com"}, false, true, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if len(*asked) != 0 {
		t.Errorf("asked %q", *asked)
	}
	if err := n.PolicyChange(t.Context(), []string{"!bad.example.com"}, false, true, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if want := "widen_parent_vm cmd=devvm network rm dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("network rm of a deny asked %q", *asked)
	}
	if !policyHas(t, e, vm, "!bad.example.com") {
		t.Error("network rm of a deny, answered no, removed it")
	}
	// In the parent folder itself, nothing asks.
	e.Dir = web
	e.Terminal = func() bool { return false }
	if err := n.SetNetwork(t.Context(), false); err != nil {
		t.Errorf("network off in the VM's own folder: %v", msg.NamesOf(err))
	}
}

// reread is vm's state as it is now.
func reread(t *testing.T, e *Env, vm string) *state.VM {
	t.Helper()
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// policyHas reports whether vm's own policy file has rule.
func policyHas(t *testing.T, e *Env, vm, rule string) bool {
	t.Helper()
	data, err := os.ReadFile(e.Dirs.VM(vm).Policy())
	if errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		t.Fatal(err)
	}
	return slices.Contains(strings.Fields(string(data)), rule)
}

// A subfolder in a repo of its own below the parent's folder (it, or a
// folder between): the other commands find no VM; up makes a new VM
// unless asked, [y/N], to open the parent's.
func TestSubfolderRepo(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := vmName(t, e)
	src := subfolder(t, e, web, "pkg/src")
	pkg := filepath.Dir(src)
	if err := os.Mkdir(filepath.Join(pkg, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, p, err := e.lookup(t.Context(), true); err != nil || p == nil || p.repo != pkg || p.f.Dir != web {
		t.Fatalf("lookup = %+v, %v", p, msg.NamesOf(err))
	}
	// A .git file (a worktree's) in the folder itself is nearer.
	if err := os.WriteFile(filepath.Join(src, ".git"), []byte("gitdir: /x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, p, err := e.lookup(t.Context(), true); err != nil || p == nil || p.repo != src {
		t.Fatalf("lookup = %+v, %v", p, msg.NamesOf(err))
	}
	if err := (Lifecycle{e}).Stop(t.Context()); msg.NamesOf(err) != "no_vm dir="+src {
		t.Errorf("Stop = %v", msg.NamesOf(err))
	}
	if c := call(f, "Stop"); c.Method != "" {
		t.Errorf("stopped %v", c.Args)
	}
	// Asked: yes opens the parent's, no makes a new VM.
	asked := answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if want := "use_parent_vm_repo dir=" + src + " parent=" + web + " repo=" + src + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if c := call(f, "Shell"); c.Args[0] != vm || c.Args[1] != src || len(vmNames(t, e)) != 1 {
		t.Errorf("Shell = %v, %d VMs", c.Args, len(vmNames(t, e)))
	}
	answers(t, e, false)
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if n := len(vmNames(t, e)); n != 2 {
		t.Errorf("%d VMs after no", n)
	}
	// Without a terminal: a new VM, nothing asked.
	subfolder(t, e, web, "pkg/docs")
	e.Terminal = func() bool { return false }
	e.Ask, e.Confirm = unasked(t), unasked(t)
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if n := len(vmNames(t, e)); n != 3 {
		t.Errorf("%d VMs without a terminal", n)
	}
}
