package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// rm asks first in a terminal and refuses without one, unless --force;
// --no-export deletes without the export.
func TestRmConfirms(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	lc := Lifecycle{e}
	vm := existingVM(t, e, Options{Agents: &[]string{"claude"}, Network: "open"}).Name()
	if err := lc.Rm(t.Context(), RmOptions{}); msg.NamesOf(err) != "no_input answer=--force" {
		t.Errorf("no terminal: %v", msg.NamesOf(err))
	}
	asked := answers(t, e, false)
	if err := lc.Rm(t.Context(), RmOptions{}); err != nil || !slices.Equal(*asked, []string{"rm_confirm vm=" + vm}) {
		t.Errorf("declined: %v, asked %q", msg.NamesOf(err), *asked)
	}
	if _, ok := f.Instances[vm]; !ok {
		t.Fatal("deleted when declined")
	}
	exported := false
	f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		exported = exported || (len(argv) == 3 && strings.Contains(argv[2], "tar -czf"))
		return nil
	}
	if err := lc.Rm(t.Context(), RmOptions{Force: true, NoExport: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; ok || exported || len(*asked) != 1 {
		t.Errorf("--force --no-export: deleted %v, exported %v, asked %q", !ok, exported, *asked)
	}
}

// rm exports an agent VM's history first; a failed export keeps the VM.
// One made for an earlier folder at this path is deleted only after
// asking, without an export.
func TestRm(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	lc := Lifecycle{e}
	st := existingVM(t, e, Options{Agents: &[]string{"claude"}, Network: "open"})
	vm := st.Name()
	f.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		if len(argv) == 3 && strings.Contains(argv[2], "tar -czf") {
			return errors.New("tar failed")
		}
		return nil
	}
	if err := lc.Rm(t.Context(), RmOptions{Force: true}); msg.NamesOf(err) != "export_failed_kept command=devvm rm vm="+vm {
		t.Errorf("Rm = %v", msg.NamesOf(err))
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
	if err := lc.Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !strings.Contains(script, "ls -d .claude .claude.json .codex") || !strings.Contains(script, "--exclude=.claude/.credentials.json") {
		t.Errorf("export script %q", script)
	}
	es, _ := os.ReadDir(e.Dirs.Exports())
	if len(es) != 2 || !strings.HasPrefix(es[1].Name(), vm+"-") || !strings.HasSuffix(es[1].Name(), ".tar.gz") {
		t.Fatalf("exports %v", es)
	}
	export := filepath.Join(e.Dirs.Exports(), es[1].Name())
	if data, _ := os.ReadFile(export); string(data) != "tarball" { //nolint:gosec // the test's own export
		t.Errorf("export = %q", data)
	}
	// The file beside it names its folder, for devvm history.
	if data, _ := os.ReadFile(paths.ExportFolder(export)); string(data) != e.Dir+"\n" {
		t.Errorf("folder = %q, want %q", data, e.Dir)
	}
	if _, ok := f.Instances[vm]; ok {
		t.Error("not deleted")
	}
	if _, err := os.Stat(e.Dirs.VM(vm).Dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("its folder: %v", err)
	}

	// Another folder at the same path.
	st = existingVM(t, e, Options{Agents: &[]string{"claude"}, Network: "open"})
	if err := set(t, st, keyFolder, "0:0"); err != nil {
		t.Fatal(err)
	}
	asked := answers(t, e, false)
	if err := lc.Rm(t.Context(), RmOptions{Force: true, NoExport: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; !ok || !slices.Equal(*asked, []string{"delete_other_folder_vm vm=" + vm + " was=" + e.Dir}) {
		t.Fatalf("declined: asked %q", *asked)
	}
	answers(t, e, true)
	script = ""
	if err := lc.Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; ok || script != "" {
		t.Errorf("confirmed: deleted %v, export %q", !ok, script)
	}
}

// rm VM checks again once it has asked: a folder that came back while it
// asked keeps its VM, and so does an agent VM stopped meanwhile, which
// its export would start.
func TestRmVMChecksAgain(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	lc := Lifecycle{e}
	vm := existingVM(t, e, Options{Agents: &[]string{"claude"}, Network: "open"}).Name()
	dir := e.Dir
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	e.Dir = t.TempDir()
	answers(t, e, true)
	fake(e).OnAsk = func(msg.M) bool { return os.Mkdir(dir, 0o700) == nil }
	if err := lc.RmVM(t.Context(), vm, RmOptions{}); msg.NamesOf(err) != "rm_vm_folder_there dir="+dir+" vm="+vm {
		t.Errorf("a folder back: %v", msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; !ok {
		t.Fatal("deleted though its folder came back")
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	fake(e).OnAsk = func(msg.M) bool {
		in := f.Instances[vm]
		in.Status = backend.StatusStopped
		f.Instances[vm] = in
		return true
	}
	if err := lc.RmVM(t.Context(), vm, RmOptions{}); msg.NamesOf(err) != "rm_gone_stopped dir="+dir+" vm="+vm {
		t.Errorf("stopped meanwhile: %v", msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; !ok {
		t.Error("deleted though it stopped")
	}
}

// rm VM refuses a VM whose state records a folder whose VM it isn't (a
// state changed by hand) as one whose folder is there: the folder it
// names, this one, keeps its own VM, which rm VM never deletes for it.
func TestRmVMFolderOfAnother(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	web := e.Dir
	own := existingVM(t, e, Options{Network: "open"}).Name()
	subfolder(t, e, filepath.Dir(web), "api")
	e.Folder = FolderHere
	if err := (Launch{e}).Up(t.Context(), Options{Network: "open"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	other, err := (finder{e}).target(t.Context(), need{})
	if err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := set(t, other.st, keyDir, web); err != nil {
		t.Fatal(err)
	}
	e.Dir, e.Folder = web, FolderAuto
	err = (Lifecycle{e}).RmVM(t.Context(), other.f.Name, RmOptions{Force: true, NoExport: true})
	if msg.NamesOf(err) != "rm_vm_folder_there dir="+web+" vm="+other.f.Name {
		t.Errorf("rm VM of another folder's state: %v", msg.NamesOf(err))
	}
	for _, vm := range []string{own, other.f.Name} {
		if _, ok := f.Instances[vm]; !ok {
			t.Errorf("%s deleted", vm)
		}
	}
}

// export saves the history to the exports folder or a file of the user's,
// which a failed export leaves as it was; the VM is kept either way.
func TestExport(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	lc := Lifecycle{e}
	vm := existingVM(t, e, Options{Network: "open"}).Name()
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
	// An exports folder open to others is the user's alone after an
	// export.
	if err := os.MkdirAll(e.Dirs.Exports(), 0o755); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	if err := os.Chmod(e.Dirs.Exports(), 0o755); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	if err := lc.Export(t.Context(), ""); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	es, _ := os.ReadDir(e.Dirs.Exports())
	if len(es) != 2 || !strings.HasPrefix(es[1].Name(), vm+"-") || !strings.HasSuffix(es[1].Name(), ".tar.gz") {
		t.Fatalf("exports %v", es)
	}
	for path, mode := range map[string]os.FileMode{
		e.Dirs.Exports(): 0o700, filepath.Join(e.Dirs.Exports(), es[1].Name()): 0o600, filepath.Join(e.Dirs.Exports(), es[0].Name()): 0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != mode {
			t.Errorf("%s: %v, want %v (%v)", path, fi.Mode().Perm(), mode, err)
		}
	}

	// A folder of the user's keeps its mode.
	mine := t.TempDir()
	if err := os.Chmod(mine, 0o755); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	if err := lc.Export(t.Context(), filepath.Join(mine, "h.tar.gz")); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if fi, _ := os.Stat(mine); fi.Mode().Perm() != 0o755 {
		t.Errorf("the user's folder: %v", fi.Mode().Perm())
	}

	dir := filepath.Join(t.TempDir(), "new")
	file := filepath.Join(dir, "h.tar.gz")
	read := func() string {
		data, _ := os.ReadFile(file) //nolint:gosec // the test's own folder
		return string(data)
	}
	if err := lc.Export(t.Context(), file); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if data := read(); data != "tarball" {
		t.Errorf("export = %q", data)
	}
	tarball = ""
	err := lc.Export(t.Context(), file)
	if le, ok := errors.AsType[*backend.LoggedError](err); !ok || msg.NamesOf(le.Msg) != "history_not_exported vm="+vm {
		t.Errorf("failed: %v", msg.NamesOf(err))
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

// An export starts a stopped VM as a launch does: its .git locked again,
// network policy's self-check run (it stops a VM that gets out directly),
// and a VM whose state has both network policy and the docker layer
// refused.
func TestExportStartsAsLaunch(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	gitEnv(t)
	gitIn(t, e.Dir, "init", "-q")
	var locks []string
	e.Protector = func(dir string) *gitprotect.Protector {
		p := gitprotect.New(dir, e.Dirs.Home, &msg.Fake{})
		p.CanLock = true
		p.Chflags = func(_ context.Context, flags, path string) error {
			locks = append(locks, flags+" "+path)
			return nil
		}
		return p
	}
	direct := false
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		switch {
		case argv[0] == "curl" && !direct:
			return errors.New("curl: (7) Failed to connect")
		case len(argv) == 3 && strings.Contains(argv[2], "tar -czf"):
			_, err := o.Stdout.Write([]byte("tarball"))
			return err
		}
		return nil
	}
	st := existingVM(t, e, Options{Network: "allowlist"})
	vm := st.Name()
	lc := Lifecycle{e}
	stop := func() {
		t.Helper()
		if err := f.Stop(t.Context(), vm); err != nil {
			t.Fatal(err)
		}
		e.Proxy.Stop(vm)
	}
	running := func() bool { return f.Instances[vm].Status == backend.StatusRunning }

	stop()
	locks = nil
	if err := lc.Export(t.Context(), ""); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !slices.Contains(locks, "uchg "+filepath.Join(e.Dir, ".git", "config")) {
		t.Errorf("locked %q", locks)
	}

	// Running, then stopped: either way the self-check stops it.
	direct = true
	if err := lc.Export(t.Context(), ""); msg.NamesOf(err) != "net_direct_route vm="+vm || running() {
		t.Errorf("running, gets out directly: %v, running %v", msg.NamesOf(err), running())
	}
	stop()
	if err := lc.Export(t.Context(), ""); msg.NamesOf(err) != "net_direct_route vm="+vm || running() {
		t.Errorf("gets out directly: %v, running %v", msg.NamesOf(err), running())
	}
	direct = false
}

// adopt gives the folder the VM made for an earlier one at its path,
// after asking; it's then opened, its .git locked as new.
func TestAdopt(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	lc, l := Lifecycle{e}, Launch{e}
	st := existingVM(t, e, Options{Network: "open"})
	if err := lc.Adopt(t.Context(), "", false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := set(t, st, keyFolder, "0:0"); err != nil {
		t.Fatal(err)
	}
	if err := l.Up(t.Context(), Options{}); msg.NamesOf(err) != "vm_other_folder vm="+st.Name()+" was="+e.Dir {
		t.Errorf("Up = %v", msg.NamesOf(err))
	}
	asked := answers(t, e, false)
	if err := lc.Adopt(t.Context(), "", false); err != nil || len(*asked) != 1 {
		t.Fatalf("declined: %v, asked %q", msg.NamesOf(err), *asked)
	}
	answers(t, e, true)
	if err := lc.Adopt(t.Context(), "", false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Errorf("Up adopted = %v", msg.NamesOf(err))
	}
	// --force adopts without asking, and without a terminal.
	if err := set(t, st, keyFolder, "0:0"); err != nil {
		t.Fatal(err)
	}
	fake(e).Terminal = false
	if err := lc.Adopt(t.Context(), "", false); msg.NamesOf(err) != "vm_other_folder_adopt vm="+st.Name()+" was="+e.Dir {
		t.Errorf("no terminal: %v", msg.NamesOf(err))
	}
	asked = answers(t, e, false)
	fake(e).Terminal = false
	if err := lc.Adopt(t.Context(), "", true); err != nil || len(*asked) != 0 {
		t.Fatalf("--force: %v, asked %q", msg.NamesOf(err), *asked)
	}
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Errorf("Up adopted with --force = %v", msg.NamesOf(err))
	}
}

// On Linux hosts, set-id files in the folder are named when a VM is made
// and once it's stopped, but not at a launch into the running VM: the
// walk is slow. Those in .git aren't.
func TestSetIDOnStop(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	e.OS = "linux"
	if err := os.MkdirAll(filepath.Join(e.Dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"tool", filepath.Join(".git", "hook")} {
		p = filepath.Join(e.Dir, p)
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, os.ModeSetuid|0o700); err != nil {
			t.Fatal(err)
		}
	}
	want := "setid_files files=" + filepath.Join(e.Dir, "tool")
	var warned msg.Recorder
	fake(e).OnWarn = warned.Add
	existingVM(t, e, Options{Network: "open"})
	if !slices.Equal(warned.Names(), []string{want}) {
		t.Errorf("a new VM: warned %q, want %s", warned.Names(), want)
	}
	warned.Reset()
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if slices.Contains(warned.Names(), want) {
		t.Errorf("a launch into the running VM named the set-id files")
	}
	warned.Reset()
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !slices.Contains(warned.Names(), want) {
		t.Errorf("stop: warned %q, want %s", warned.Names(), want)
	}
}
