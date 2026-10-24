package gitprotect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// fixture is a project folder with a protector whose chflags and warnings
// are recorded, and a VM state for it.
type fixture struct {
	t     *testing.T
	root  string // the test's folder, physical
	dir   string // the project folder
	p     *Protector
	vm    *state.VM
	flags []string // "uchg PATH" per chflags call
	warns []string // names mode
}

func newFixture(t *testing.T, name string) *fixture {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, dir: filepath.Join(root, name)}
	mkdirs(t, f.dir)
	f.p = &Protector{
		Dir: f.dir, Home: filepath.Join(root, "home"), OS: "darwin",
		Chflags: func(_ context.Context, flags, path string) error {
			f.flags = append(f.flags, flags+" "+path)
			return nil
		},
		Warn: func(m msg.M) { f.warns = append(f.warns, m.Names()) },
	}
	f.vm, err = state.CreateVM(paths.Dirs{Data: filepath.Join(root, "data")}, "devvm-p-0123456789abcd", state.NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) path(rel string) string { return filepath.Join(f.dir, rel) }

func (f *fixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.CommandContext(f.t.Context(), "git", append([]string{ //nolint:gosec // the test's own
		"-c", "user.email=a@b", "-c", "user.name=a",
		"-c", "init.defaultBranch=main", "-c", "protocol.file.allow=always",
	}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *fixture) warned(want string) bool {
	return slices.ContainsFunc(f.warns, func(w string) bool { return strings.Contains(w, want) })
}

func (f *fixture) locked(path string) bool { return slices.Contains(f.flags, "uchg "+path) }

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
}

func TestPathsUnderSpaces(t *testing.T) {
	f := newFixture(t, "a b")
	mkdirs(t, f.path(".git/hooks"), f.path(".git/modules/s m/hooks"), f.path(".git/modules/s m/objects"), f.path(".git/modules/s m/refs"))
	write(t, f.path(".git/config"), "")
	write(t, f.path(".git/modules/s m/config"), "")
	write(t, f.path(".git/modules/s m/HEAD"), "")
	got := f.p.paths(t.Context(), f.p.embedded(t.Context()))
	for _, want := range []string{".git/hooks", ".git/config", ".git/modules/s m/hooks", ".git/modules/s m/config"} {
		if !slices.Contains(got, f.path(want)) {
			t.Errorf("paths = %q, missing %s", got, want)
		}
	}
}

func TestPathsWorktrees(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git/hooks"), f.path(".git/worktrees/w"))
	write(t, f.path(".git/config"), "")
	write(t, f.path(".git/worktrees/w/commondir"), "../..\n")
	got := f.p.paths(t.Context(), embeds{})
	if !slices.Contains(got, f.path(".git/worktrees/w/commondir")) || slices.ContainsFunc(got, func(p string) bool {
		return strings.HasSuffix(p, "config.worktree")
	}) {
		t.Errorf("paths = %q", got)
	}
	f.git(f.dir, "config", "--file", f.path(".git/config"), "extensions.worktreeConfig", "true")
	got = f.p.paths(t.Context(), embeds{})
	for _, want := range []string{".git/config.worktree", ".git/worktrees/w/config.worktree"} {
		if !slices.Contains(got, f.path(want)) {
			t.Errorf("with worktreeConfig: paths = %q, missing %s", got, want)
		}
	}
}

// A linked worktree's .git file is all there is to lock.
func TestPathsLinkedWorktree(t *testing.T) {
	f := newFixture(t, "p")
	write(t, f.path(".git"), "gitdir: /elsewhere/.git/worktrees/p\n")
	if got := f.p.paths(t.Context(), embeds{}); !slices.Equal(got, []string{f.path(".git")}) {
		t.Errorf("paths = %q", got)
	}
}

// Nothing is created or locked through a symlink the VM planted under .git.
func TestLockNeverThroughSymlinks(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	out := filepath.Join(f.root, "outside")
	mkdirs(t, f.path(".git/worktrees/x"), out)
	f.git(f.dir, "config", "--file", f.path(".git/config"), "extensions.worktreeConfig", "true")
	symlink(t, filepath.Join(out, "marker"), f.path(".git/worktrees/x/config.worktree")) // dangling
	symlink(t, out, f.path(".git/worktrees/y"))
	write(t, filepath.Join(out, "commondir"), "")
	if err := f.p.lock(t.Context(), f.vm, embeds{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(out, "marker"), filepath.Join(out, "config.worktree")} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("%s was created", p)
		}
	}
	for _, want := range []string{
		"git_not_locked path=" + f.path(".git/worktrees/x/config.worktree"),
		"git_not_locked path=" + f.path(".git/worktrees/y/commondir"),
	} {
		if !f.warned(want) {
			t.Errorf("warnings %q lack %s", f.warns, want)
		}
	}
	if f.warned("path="+f.path(".git/config")+" ") || !f.locked(f.path(".git/config")) {
		t.Errorf("the real .git/config: warnings %q, locked %q", f.warns, f.flags)
	}
	for _, fl := range f.flags {
		if strings.Contains(fl, out) {
			t.Errorf("locked outside the project: %s", fl)
		}
	}
}

// A missing hooks folder (and config.worktree) is created, then locked, so
// the VM can't make one.
func TestLockCreatesMissingHooks(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	f.git(f.root, "init", "-q", "--template=", f.dir)
	if err := os.RemoveAll(f.path(".git/hooks")); err != nil {
		t.Fatal(err)
	}
	mkdirs(t, f.path(".git/modules/s/objects"), f.path(".git/modules/s/refs"))
	write(t, f.path(".git/modules/s/HEAD"), "")
	write(t, f.path(".git/modules/s/config"), "")
	if err := f.p.lock(t.Context(), f.vm, embeds{}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".git/hooks", ".git/modules/s/hooks"} {
		if fi, err := os.Stat(f.path(p)); err != nil || !fi.IsDir() || !f.locked(f.path(p)) {
			t.Errorf("%s: %v, locks %q", p, err, f.flags)
		}
	}
	if got, want := f.vm.Get(KeyGitInode), inode(f.path(".git")); got == "" || got != want {
		t.Errorf("git_inode = %q, want %q", got, want)
	}
}

// A symlinked .git/config is reported, not locked; the hooks are.
func TestLockOnlyRealPaths(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git/hooks"))
	write(t, f.path("cfg"), "[core]\n")
	symlink(t, "../cfg", f.path(".git/config"))
	if err := f.p.lock(t.Context(), f.vm, embeds{}); err != nil {
		t.Fatal(err)
	}
	if !f.warned("git_not_locked path="+f.path(".git/config")) || f.locked(f.path(".git/config")) || f.locked(f.path("cfg")) {
		t.Errorf("warnings %q, locks %q", f.warns, f.flags)
	}
	if !f.locked(f.path(".git/hooks")) {
		t.Errorf("hooks not locked: %q", f.flags)
	}
}

// .git a symlink to another repo: nothing of that repo is locked. A
// commondir's target is shown without its control characters.
func TestNewVMOtherRepoAndCommondir(t *testing.T) {
	f := newFixture(t, "p")
	other := filepath.Join(f.root, "other")
	mkdirs(t, filepath.Join(other, ".git", "hooks"))
	write(t, filepath.Join(other, ".git", "config"), "")
	symlink(t, filepath.Join(other, ".git"), f.path(".git"))
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	if !f.warned("git_not_locked path=" + f.path(".git/config")) {
		t.Errorf("warnings %q", f.warns)
	}
	for _, fl := range f.flags {
		if strings.Contains(fl, other) {
			t.Errorf("locked the other repo: %s", fl)
		}
	}
	if err := os.Remove(f.path(".git")); err != nil {
		t.Fatal(err)
	}
	mkdirs(t, f.path(".git"))
	write(t, f.path(".git/commondir"), "../x\x1b[2J\n")
	f.warns = nil
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	if !f.warned(`git_commondir git=`+f.path(".git")+` target="../x\x1b[2J"`) || f.warned("\x1b") {
		t.Errorf("warnings %q", f.warns)
	}
}

// embeddedRepo makes a superproject at f.dir with a gitlink sub whose git
// dir is sub/.git, and another, rel, whose .git file points at a git dir
// elsewhere in the working tree.
func embeddedRepo(f *fixture) {
	f.t.Helper()
	f.git(f.dir, "init", "-q")
	f.git(f.dir, "commit", "-q", "--allow-empty", "-m", "x")
	mkdirs(f.t, f.path("sub"), f.path("rel"), f.path(".gitdirs"))
	f.git(f.path("sub"), "init", "-q")
	f.git(f.path("sub"), "commit", "-q", "--allow-empty", "-m", "s")
	f.git(f.dir, "update-index", "--add", "--cacheinfo", "160000,"+f.git(f.path("sub"), "rev-parse", "HEAD")+",sub")
	f.git(f.dir, "init", "-q", "--separate-git-dir="+f.path(".gitdirs/rel"), "rel")
	f.git(f.path("rel"), "commit", "-q", "--allow-empty", "-m", "r")
	f.git(f.dir, "update-index", "--add", "--cacheinfo", "160000,"+f.git(f.path("rel"), "rev-parse", "HEAD")+",rel")
}

func TestEmbedded(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	embeddedRepo(f)
	emb := f.p.embedded(t.Context())
	if !slices.Equal(emb.inside, []string{f.path(".gitdirs/rel"), f.path("sub/.git")}) &&
		!slices.Equal(emb.inside, []string{f.path("sub/.git"), f.path(".gitdirs/rel")}) {
		t.Errorf("embedded = %q", emb.inside)
	}
	got := f.p.paths(t.Context(), emb)
	for _, want := range []string{"sub/.git/config", ".gitdirs/rel/hooks"} {
		if !slices.Contains(got, f.path(want)) {
			t.Errorf("paths = %q, missing %s", got, want)
		}
	}
	// A nested submodule's git dir, from the submodule's own index.
	mkdirs(t, f.path("sub/inner"))
	f.git(f.path("sub/inner"), "init", "-q")
	f.git(f.path("sub/inner"), "commit", "-q", "--allow-empty", "-m", "i")
	f.git(f.path("sub"), "update-index", "--add", "--cacheinfo", "160000,"+f.git(f.path("sub/inner"), "rev-parse", "HEAD")+",inner")
	if emb := f.p.embedded(t.Context()); !slices.Contains(emb.inside, f.path("sub/inner/.git")) {
		t.Errorf("embedded = %q, without the nested one", emb.inside)
	}
}

// A .git/modules git dir with a symlinked HEAD counts; its logs folder
// doesn't.
func TestGitDirs(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	embeddedRepo(f)
	m := f.path(".git/modules/evil")
	mkdirs(t, f.path(".git/modules"))
	if out, err := exec.CommandContext(t.Context(), "cp", "-R", f.path("sub/.git"), m).CombinedOutput(); err != nil { //nolint:gosec // the test's own
		t.Fatalf("cp: %v %s", err, out)
	}
	if err := os.Remove(filepath.Join(m, "HEAD")); err != nil {
		t.Fatal(err)
	}
	symlink(t, "refs/heads/main", filepath.Join(m, "HEAD"))
	got := f.p.gitDirs()
	if !slices.Contains(got, m) || slices.Contains(got, filepath.Join(m, "logs")) {
		t.Errorf("gitDirs = %q", got)
	}
}

// Includes and hook symlinks into the project folder are reported when the
// VM is made, and their hash recorded.
func TestNewVMDelegations(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	f.git(f.dir, "init", "-q")
	cfg := f.path(".git/config")
	f.git(f.dir, "config", "--file", cfg, "include.path", "../shared.gitconfig")
	f.git(f.dir, "config", "--file", cfg, "includeIf.gitdir:~/x/.path", "../later/x.gitconfig") // not there yet
	f.git(f.dir, "config", "--file", cfg, "--add", "include.path", filepath.Join(f.p.Home, "outside.gitconfig"))
	mkdirs(t, f.path("scripts"), f.path(".git/hooks"))
	symlink(t, "../../scripts/pre-commit", f.path(".git/hooks/pre-commit"))
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"git_include_inside config=" + cfg + " path=../shared.gitconfig",
		"git_include_inside config=" + cfg + " path=../later/x.gitconfig",
		"git_hook_link_inside hook=" + f.path(".git/hooks/pre-commit") + " target=../../scripts/pre-commit",
	} {
		if !f.warned(want) {
			t.Errorf("warnings %q lack %s", f.warns, want)
		}
	}
	if f.warned("outside.gitconfig") {
		t.Errorf("an include outside the project was reported: %q", f.warns)
	}
	if f.vm.Get(KeyGitDelegations) == delegationsSum(nil) {
		t.Error("git_delegations is the empty list's")
	}
}

func TestNewVMGitdirOutside(t *testing.T) {
	needGit(t)
	f := newFixture(t, "p")
	embeddedRepo(f)
	out := filepath.Join(f.root, "elsewhere")
	mkdirs(t, out)
	f.git(out, "init", "-q", "--bare", "sub.git")
	if err := os.RemoveAll(f.path("sub/.git")); err != nil {
		t.Fatal(err)
	}
	write(t, f.path("sub/.git"), "gitdir: "+filepath.Join(out, "sub.git")+"\n")
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	if want := "git_gitdir_outside git=" + filepath.Join(out, "sub.git") + " sub=" + f.path("sub"); !f.warned(want) {
		t.Errorf("warnings %q lack %s", f.warns, want)
	}
	if got := f.vm.Get(KeyGitEmbedded); got != `[".gitdirs/rel"]` {
		t.Errorf("git_embedded = %s", got)
	}
}

func TestWorktreeStrays(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git/worktrees/ok"), f.path(".git/worktrees/moved"), f.path(".git/worktrees/gone"), f.path("evil"))
	write(t, f.path(".git/worktrees/ok/commondir"), "../..\n")
	write(t, f.path(".git/worktrees/moved/commondir"), "../../../evil\n")
	want := []string{f.path(".git/worktrees/gone"), f.path(".git/worktrees/moved")}
	if got := f.p.worktreeStrays(); !slices.Equal(got, want) {
		t.Errorf("worktreeStrays = %q, want %q", got, want)
	}
}

func TestHooksPathInside(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".husky"), filepath.Join(f.root, "other"))
	f.p.Home = f.root
	for _, tc := range []struct {
		path string
		want bool
	}{
		{".husky", true},
		{f.path(".husky"), true},
		{f.path("missing/hooks"), true},
		{filepath.Join(f.root, "other", "..", "p", ".husky"), true},
		{"~/p/.husky", true},
		{filepath.Join(f.root, "other"), false},
		{f.dir + "-sibling/hooks", false},
	} {
		if got := f.p.hooksPathInside(tc.path); got != tc.want {
			t.Errorf("hooksPathInside(%s) = %v", tc.path, got)
		}
	}
}

// A path that moved while it was locked (a swapped symlink) is unlocked
// again and reported.
func TestFlagRaced(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git"))
	cfg := f.path(".git/config")
	write(t, cfg, "[core]\n")
	f.p.Chflags = func(_ context.Context, flags, path string) error {
		f.flags = append(f.flags, flags+" "+path)
		if flags == "uchg" {
			if err := os.Rename(cfg, cfg+".old"); err != nil {
				t.Fatal(err)
			}
			write(t, cfg, "x")
		}
		return nil
	}
	if f.p.flag(t.Context(), "uchg", cfg) {
		t.Error("flag succeeded")
	}
	if !slices.Equal(f.flags, []string{"uchg " + cfg, "nouchg " + cfg}) || !f.warned("git_lock_raced path="+cfg) {
		t.Errorf("flags %q, warnings %q", f.flags, f.warns)
	}
}

// Unlock takes off what lock put on, never through a symlink; on hosts
// without locks it does nothing.
func TestUnlock(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git/hooks"))
	other := filepath.Join(f.root, "other")
	write(t, other, "")
	symlink(t, other, f.path(".git/config"))
	f.p.Unlock(t.Context())
	if !slices.Equal(f.flags, []string{"nouchg " + f.path(".git/hooks")}) {
		t.Errorf("flags %q", f.flags)
	}
	f.flags = nil
	f.p.OS = "linux"
	f.p.Unlock(t.Context())
	if len(f.flags) > 0 {
		t.Errorf("linux: flags %q", f.flags)
	}
}

// Off: nothing is locked. On hosts without locks it's off, saying so when
// there's a .git.
func TestNewVMOffAndLinux(t *testing.T) {
	f := newFixture(t, "p")
	mkdirs(t, f.path(".git/hooks"))
	if err := f.p.NewVM(t.Context(), f.vm, true); err != nil {
		t.Fatal(err)
	}
	if f.vm.Get(KeyGitProtect) != "off" || len(f.flags) > 0 || len(f.warns) > 0 {
		t.Errorf("off: %s, locks %q, warnings %q", f.vm.Get(KeyGitProtect), f.flags, f.warns)
	}
	f.p.OS = "linux"
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	if f.vm.Get(KeyGitProtect) != "off" || len(f.flags) > 0 || !slices.Equal(f.warns, []string{"git_protect_needs_macos_new"}) {
		t.Errorf("linux: %s, locks %q, warnings %q", f.vm.Get(KeyGitProtect), f.flags, f.warns)
	}
	if err := os.RemoveAll(f.path(".git")); err != nil {
		t.Fatal(err)
	}
	f.warns = nil
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil || len(f.warns) > 0 {
		t.Errorf("linux without .git: %v, warnings %q", err, f.warns)
	}
	f.p.OS = "darwin"
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil || f.vm.Get(KeyGitProtect) != "on" || len(f.flags) > 0 {
		t.Errorf("no .git: %v, %s, locks %q", err, f.vm.Get(KeyGitProtect), f.flags)
	}
}

// The real flag, on macOS: the VM's user can't add a hook or change the
// config, nor make a hooks folder the repo didn't have.
func TestRealLocks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("chflags is macOS's")
	}
	needGit(t)
	f := newFixture(t, "p")
	t.Cleanup(func() { _ = exec.Command("chflags", "-R", "nouchg", f.root).Run() }) //nolint:noctx,gosec // cleanup, of the test's own folder
	f.p.Chflags = New(f.dir).Chflags
	f.git(f.root, "init", "-q", "--template=", f.dir)
	if err := f.p.NewVM(t.Context(), f.vm, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(f.path(".git/hooks/x"), 0o700); err == nil {
		t.Error("made a folder in the locked hooks")
	}
	if err := os.WriteFile(f.path(".git/hooks/pre-commit"), []byte("x"), 0o600); err == nil {
		t.Error("added a hook")
	}
	if err := os.WriteFile(f.path(".git/config"), []byte("x"), 0o600); err == nil {
		t.Error("changed the config")
	}
	if len(f.warns) > 0 {
		t.Errorf("warnings %q", f.warns)
	}
}
