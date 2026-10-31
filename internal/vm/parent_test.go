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
	"github.com/w3cj/devvm/internal/msg"
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
	own, parent, err := e.lookup(t.Context(), "", true)
	if err != nil {
		t.Fatal(names(err))
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
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	lib := subfolder(t, e, web, "src/lib")
	if dir, sub := lookupVM(t, e); dir != web || sub != lib {
		t.Errorf("from src/lib: %q for %q, want web's", dir, sub)
	}
	// The nearest one wins.
	subfolder(t, e, web, "src")
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Here: true}); err != nil {
		t.Fatal(names(err))
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
	own, _, err := e.lookup(t.Context(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	in := f.Instances[own.f.Name]
	delete(f.Instances, own.f.Name)
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("from src with its state alone: %q, want its own", dir)
	}
	f.Instances[own.f.Name] = in
	// One made for an earlier folder at its path is passed over.
	e.Dir = lib
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
	if dir, _ := lookupVM(t, e); dir != web {
		t.Errorf("past another folder's VM: %q, want web's", dir)
	}
	// Never the home folder's, or one above it.
	e.Home = web
	if dir, _ := lookupVM(t, e); dir != "" {
		t.Errorf("with web as home: %q", dir)
	}
	// Without parents, only its own.
	e.Home = filepath.Dir(web)
	if _, p, err := e.lookup(t.Context(), "", false); err != nil || p != nil {
		t.Errorf("without parents: %+v, %v", p, err)
	}
}

func vmNames(t *testing.T, e *Env) []string {
	t.Helper()
	es, err := os.ReadDir(e.Dirs.VMs())
	if err != nil {
		t.Fatal(err)
	}
	var ns []string
	for _, x := range es {
		ns = append(ns, x.Name())
	}
	return ns
}

// up in a subfolder opens the parent's VM there: without a terminal, said
// in an info line; in one, after asking, a no making a new VM; with
// --here, a new VM without asking.
func TestUpInSubfolder(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	var infos []string
	e.Info = func(m msg.M) { infos = append(infos, m.Names()) }
	src := subfolder(t, e, web, "src")
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	vm := vmName(t, e)
	if c := call(f, "Shell"); c.Args[0] != vm || c.Args[1] != src {
		t.Errorf("Shell = %v, want %s in %s", c.Args, vm, src)
	}
	if !slices.Contains(infos, "using_parent_vm dir="+web+" vm="+vm) {
		t.Errorf("infos = %q", infos)
	}
	// Asked: yes opens it, no makes a new VM.
	asked := answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if want := "use_parent_vm dir=" + src + " parent=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if n := len(vmNames(t, e)); n != 1 {
		t.Errorf("%d VMs after yes", n)
	}
	answers(t, e, false)
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
	}
	if n := len(vmNames(t, e)); n != 2 {
		t.Errorf("%d VMs after no", n)
	}
	// --clone: as --here, a new VM's checks (this subfolder isn't a
	// repo), nothing asked.
	subfolder(t, e, web, "tests")
	asked = answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Clone: true}); err == nil ||
		strings.HasPrefix(names(err), "clone_needs_new_vm") || len(*asked) != 0 {
		t.Errorf("--clone: %v, asked %q", names(err), *asked)
	}
	// --here: a new VM, nothing asked.
	subfolder(t, e, web, "docs")
	asked = answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Here: true}); err != nil {
		t.Fatal(names(err))
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
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	src := subfolder(t, e, web, "src")
	_, p, err := e.lookup(t.Context(), "", true)
	if err != nil || p == nil {
		t.Fatalf("lookup = %+v, %v", p, names(err))
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
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := vmName(t, e)
	var infos []string
	e.Info = func(m msg.M) { infos = append(infos, m.Names()) }
	subfolder(t, e, web, "src")
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	if want := []string{"using_parent_vm dir=" + web + " vm=" + vm, "stopped vm=" + vm}; !slices.Equal(infos, want) {
		t.Errorf("infos = %q, want %q", infos, want)
	}
	if c := call(f, "Stop"); c.Args[0] != vm {
		t.Errorf("Stop = %v", c.Args)
	}
	if err := (Lifecycle{e}).Adopt(t.Context()); names(err) != "no_vm dir="+e.Dir {
		t.Errorf("Adopt = %v", names(err))
	}
	// rm: refused without a terminal, -f or not; asked in one, a no
	// keeping it.
	infos = nil
	if err := (Lifecycle{e}).Rm(t.Context(), true); names(err) != "rm_parent_vm dir="+web+" vm="+vm {
		t.Errorf("Rm without a terminal = %v", names(err))
	}
	if len(infos) == 0 || infos[0] != "using_parent_vm dir="+web+" vm="+vm {
		t.Errorf("Rm's infos = %q", infos)
	}
	asked := answers(t, e, false)
	if err := (Lifecycle{e}).Rm(t.Context(), true); err != nil {
		t.Fatal(names(err))
	}
	if n := len(vmNames(t, e)); n != 1 {
		t.Errorf("%d VMs after no", n)
	}
	if want := "delete_parent_vm dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	asked = answers(t, e, true)
	if err := (Lifecycle{e}).Rm(t.Context(), true); err != nil {
		t.Fatal(names(err))
	}
	if want := "delete_parent_vm dir=" + web + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if n := len(vmNames(t, e)); n != 0 {
		t.Errorf("%d VMs after rm", n)
	}
}

// A subfolder in a repo of its own below the parent's folder (it, or a
// folder between): the other commands find no VM; up makes a new VM
// unless asked, [y/N], to open the parent's.
func TestSubfolderRepo(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := vmName(t, e)
	src := subfolder(t, e, web, "pkg/src")
	pkg := filepath.Dir(src)
	if err := os.Mkdir(filepath.Join(pkg, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, p, err := e.lookup(t.Context(), "", true); err != nil || p == nil || p.repo != pkg || p.f.Dir != web {
		t.Fatalf("lookup = %+v, %v", p, names(err))
	}
	// A .git file (a worktree's) in the folder itself is nearer.
	if err := os.WriteFile(filepath.Join(src, ".git"), []byte("gitdir: /x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, p, err := e.lookup(t.Context(), "", true); err != nil || p == nil || p.repo != src {
		t.Fatalf("lookup = %+v, %v", p, names(err))
	}
	if err := (Lifecycle{e}).Stop(t.Context()); names(err) != "no_vm dir="+src {
		t.Errorf("Stop = %v", names(err))
	}
	if c := call(f, "Stop"); c.Method != "" {
		t.Errorf("stopped %v", c.Args)
	}
	// Asked: yes opens the parent's, no makes a new VM.
	asked := answers(t, e, true)
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if want := "use_parent_vm_repo dir=" + src + " parent=" + web + " repo=" + src + " vm=" + vm; !slices.Equal(*asked, []string{want}) {
		t.Errorf("asked %q, want %q", *asked, want)
	}
	if c := call(f, "Shell"); c.Args[0] != vm || c.Args[1] != src || len(vmNames(t, e)) != 1 {
		t.Errorf("Shell = %v, %d VMs", c.Args, len(vmNames(t, e)))
	}
	answers(t, e, false)
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
	}
	if n := len(vmNames(t, e)); n != 2 {
		t.Errorf("%d VMs after no", n)
	}
	// Without a terminal: a new VM, nothing asked.
	subfolder(t, e, web, "pkg/docs")
	e.Terminal = func() bool { return false }
	e.Ask, e.Confirm = unasked(t), unasked(t)
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
	}
	if n := len(vmNames(t, e)); n != 3 {
		t.Errorf("%d VMs without a terminal", n)
	}
}
