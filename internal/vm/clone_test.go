package vm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/project"
)

// gitEnv gives git in the test a config and identity of its own.
func gitEnv(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_NOSYSTEM": "1",
		"GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@t",
	} {
		t.Setenv(k, v)
	}
}

// gitIn runs git in dir, failing the test if it fails, and returns its
// output, trimmed.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...).CombinedOutput() //nolint:gosec // the test's own
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// cloneEnv is upEnv with the folder a repo with a commit on main, and a
// VM whose files are vmfs: the fake backend runs a clone's git there, and
// ssh is the ssh stub, which pushes and fetches there too.
func cloneEnv(t *testing.T) (e *Env, f *backendtest.Fake, vmfs string) {
	t.Helper()
	e, f, _ = upEnv(t, allNative())
	gitEnv(t)
	gitIn(t, e.Dir, "init", "-q", "-b", "main")
	gitIn(t, e.Dir, "commit", "-q", "--allow-empty", "-m", "one")
	stubDir := t.TempDir()
	vmfs = filepath.Join(stubDir, "vmfs")
	bin := t.TempDir()
	stub, err := filepath.Abs(filepath.Join("..", "..", "testdata", "stubs", "ssh"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stub, filepath.Join(bin, "ssh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_DIR", stubDir)
	t.Setenv("STUB_LOG", filepath.Join(stubDir, "calls"))
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		var cmd *exec.Cmd
		switch {
		case len(argv) > 3 && argv[0] == "git" && argv[1] == "-C":
			cmd = exec.CommandContext(context.Background(), "git", append([]string{"-C", vmfs + argv[2]}, argv[3:]...)...) //nolint:gosec // the test's own
		case len(argv) == 6 && argv[0] == "sh" && strings.Contains(argv[2], "receive.denyCurrentBranch"):
			cmd = exec.CommandContext(context.Background(), "bash", "-c", `sudo() { "$@"; }; `+argv[2], argv[3], vmfs+argv[4], argv[5]) //nolint:gosec // the test's own
		default:
			return nil
		}
		cmd.Stdout, cmd.Stderr = o.Stdout, o.Stderr
		return cmd.Run()
	}
	return e, f, vmfs
}

// A --clone VM: nothing of the folder's mounted, nor .git protected; its
// clone has the host's branches with the host's checked out, and the
// repo a devvm remote for it. Opened again, it isn't copied again.
func TestUpClone(t *testing.T) {
	e, f, vmfs := cloneEnv(t)
	gitIn(t, e.Dir, "branch", "other")
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Clone: true})
	vm := st.Name()
	if spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec); spec.Project != "" || spec.Tools == "" {
		t.Errorf("spec %+v", spec)
	}
	for k, want := range map[string]string{KeyMode: ModeClone, KeySeeded: "yes", KeyCloneRemote: "devvm", gitprotect.KeyGitProtect: ""} {
		if got := st.Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	if got := gitIn(t, e.Dir, "remote", "get-url", "devvm"); got != vm+":"+e.Dir {
		t.Errorf("remote = %q", got)
	}
	clone := vmfs + e.Dir
	if got := gitIn(t, clone, "branch", "--format=%(refname:short)"); got != "main\nother" {
		t.Errorf("clone's branches %q", got)
	}
	if got := gitIn(t, clone, "symbolic-ref", "--short", "HEAD"); got != "main" {
		t.Errorf("clone's HEAD %q", got)
	}
	if err := e.Up(t.Context(), Options{Clone: true}); err != nil {
		t.Fatal(names(err))
	}
	calls, _ := os.ReadFile(os.Getenv("STUB_LOG")) //nolint:gosec // the test's own
	if n := strings.Count(string(calls), "git-receive-pack"); n != 1 {
		t.Errorf("pushed %d times:\n%s", n, calls)
	}
}

// rm on a --clone VM: branches with commits the repo lacks are offered
// for fetching, uncommitted changes in the clone are asked about, and
// the remote goes with the VM. A VM's answer that isn't an object name
// is never looked up.
func TestCloneRm(t *testing.T) {
	e, f, vmfs := cloneEnv(t)
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Clone: true})
	vm, clone := st.Name(), vmfs+e.Dir
	gitIn(t, clone, "commit", "-q", "--allow-empty", "-m", "from-vm")

	// Not fetched, then dirty and kept.
	asked := answers(t, e, false)
	if err := os.WriteFile(filepath.Join(clone, "new"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	want := []string{"fetch_before_delete prefix=saved/devvm", "delete_anyway"}
	if _, ok := f.Instances[vm]; !ok || !slices.Equal(*asked, want) {
		t.Fatalf("declined: asked %q", *asked)
	}
	if _, err := hostGit(t.Context(), e.Dir, "rev-parse", "-q", "--verify", "refs/heads/saved/devvm/main"); err == nil {
		t.Error("fetched without a yes")
	}

	// Stopped: started for the check. Fetched, then deleted.
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(clone, "new")); err != nil {
		t.Fatal(err)
	}
	asked = answers(t, e, true)
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	if _, ok := f.Instances[vm]; ok || !slices.Equal(*asked, want[:1]) {
		t.Errorf("confirmed: deleted %v, asked %q", !ok, *asked)
	}
	if got := gitIn(t, e.Dir, "log", "--format=%s", "-1", "refs/heads/saved/devvm/main"); got != "from-vm" {
		t.Errorf("fetched %q", got)
	}
	if out, err := hostGit(t.Context(), e.Dir, "remote"); err != nil || strings.Contains(out, "devvm") {
		t.Errorf("remotes %q", out)
	}
	if out := gitIn(t, e.Dir, "for-each-ref", "--format=%(refname)", "refs/remotes"); out != "" {
		t.Errorf("remote-tracking branches %q", out)
	}

	// Known commits on any ref are fetched; unknown, dangling or not
	// object names aren't.
	dangling := gitIn(t, e.Dir, "commit-tree", "-m", "dangling", gitIn(t, e.Dir, "rev-parse", "HEAD^{tree}"))
	refs := []byte("0123456789abcdef0123456789abcdef01234567 refs/heads/known\n$(touch-x) refs/heads/evil\nnot-hex\n\n" +
		gitIn(t, e.Dir, "rev-parse", "HEAD") + " refs/heads/main\n" + dangling + " refs/heads/dangling\n")
	got := unfetchedRefs(t.Context(), e.Dir, refs)
	if !slices.Equal(got, []string{"known", "evil", "", "dangling"}) {
		t.Errorf("unfetched %q", got)
	}
}

// Branch names git refuses are skipped, and never asked about.
func TestFetchUnfetchedInvalid(t *testing.T) {
	e, _, _ := cloneEnv(t)
	got := printed(t, e, true)
	if err := e.fetchUnfetched(t.Context(), "devvm-web-0123456789abcd", e.Dir, "devvm", []string{"a..b", "x y", "-x:refs/heads/main"}); err != nil {
		t.Fatal(names(err))
	}
	want := []string{`branch_names_invalid refs="a..b" "x y" "-x:refs/heads/main"`}
	if !slices.Equal(*got, want) {
		t.Errorf("printed %q", *got)
	}
	gitIn(t, e.Dir, "branch", "saved")
	*got = nil
	err := e.fetchUnfetched(t.Context(), "devvm-web-0123456789abcd", e.Dir, "devvm", []string{"main"})
	if names(err) != "saved_names_taken remote=devvm vm=devvm-web-0123456789abcd" || len(*got) > 0 {
		t.Errorf("no free names: %s, printed %q", names(err), *got)
	}
}

// A deleted VM's branches go under saved/<remote>, else saved/<remote>-N,
// the first no branch is, is in or holds.
func TestSavedPrefix(t *testing.T) {
	gitEnv(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	for _, c := range []struct{ add, want string }{
		{"", "saved/devvm"},
		{"saved/devvm/main", "saved/devvm-2"},
		{"saved/devvm-2", "saved/devvm-3"},
		{"saved/devvm-30/x", "saved/devvm-3"},
		{"saved/devvm-3/a/b", "saved/devvm-4"},
		{"saved", ""},
	} {
		if c.add != "" {
			if c.add == "saved" {
				for _, b := range strings.Fields(gitIn(t, dir, "for-each-ref", "--format=%(refname)", "refs/heads/saved")) {
					gitIn(t, dir, "update-ref", "-d", b)
				}
			}
			gitIn(t, dir, "branch", c.add)
		}
		if got, _ := savedPrefix(t.Context(), dir, "devvm"); got != c.want {
			t.Errorf("after %s: %q, want %q", c.add, got, c.want)
		}
	}
}

// --clone needs the top of a repo, a commit and a branch.
func TestCheckCloneRepo(t *testing.T) {
	gitEnv(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	check := func(d string) string { return names(checkCloneRepo(t.Context(), d)) }
	if got := check(dir); got != "clone_not_top dir="+dir {
		t.Errorf("no repo: %s", got)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	if got := check(sub); got != "clone_not_top dir="+sub {
		t.Errorf("subfolder: %s", got)
	}
	if got := check(dir); got != "clone_no_commits" {
		t.Errorf("no commits: %s", got)
	}
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, dir, "checkout", "-q", "--detach")
	if got := check(dir); got != "clone_detached" {
		t.Errorf("detached: %s", got)
	}
	gitIn(t, dir, "checkout", "-q", "main")
	if got := check(dir); got != "" {
		t.Errorf("a branch: %s", got)
	}
}

// The repo's remote for the VM: devvm, devvm-<folder>, the VM's name, the
// first that's free or already the VM's.
func TestFreeRemote(t *testing.T) {
	gitEnv(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	f := project.Folder{Dir: dir, Name: "devvm-web-0123456789abcd"}
	url := cloneURL(f.Name, dir)
	for _, c := range []struct {
		add  [2]string
		want string
	}{
		{want: "devvm"},
		{add: [2]string{"devvm", "https://example.com/a"}, want: "devvm-web"},
		{add: [2]string{"devvm-web", url}, want: "devvm-web"},
		{add: [2]string{"devvm-web", "https://example.com/b"}, want: f.Name},
		{add: [2]string{f.Name, "https://example.com/c"}, want: "error remotes_taken slug=web vm=" + f.Name},
	} {
		if c.add[0] != "" {
			_, _ = hostGit(t.Context(), dir, "remote", "remove", c.add[0])
			gitIn(t, dir, "remote", "add", c.add[0], c.add[1])
		}
		got, err := freeRemote(t.Context(), f)
		if err != nil {
			got = "error " + names(err)
		}
		if got != c.want {
			t.Errorf("after %v: %q, want %q", c.add, got, c.want)
		}
	}
}

// The remote goes only while it's still the VM's, with its upstream
// settings and remote-tracking branches; those with commits no other ref
// has are kept as new local branches first, never over existing ones.
func TestRemoveRemote(t *testing.T) {
	gitEnv(t)
	e := &Env{}
	got := printed(t, e, false)
	dir := t.TempDir()
	vm := "devvm-web-0123456789abcd"
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, dir, "remote", "add", "devvm", "https://example.com/mine")
	e.removeRemote(t.Context(), dir, vm, "devvm")
	gitIn(t, dir, "remote", "get-url", "devvm")
	gitIn(t, dir, "remote", "set-url", "devvm", cloneURL(vm, dir))

	// Fetched: main as the host has it, feature/x and only/y with their
	// own commits, y's under x's. saved/devvm is taken.
	one := gitIn(t, dir, "rev-parse", "HEAD")
	tree := gitIn(t, dir, "rev-parse", "HEAD^{tree}")
	x := gitIn(t, dir, "commit-tree", "-p", one, "-m", "x", tree)
	y := gitIn(t, dir, "commit-tree", "-p", x, "-m", "y", tree)
	gitIn(t, dir, "update-ref", "refs/remotes/devvm/main", one)
	gitIn(t, dir, "update-ref", "refs/remotes/devvm/feature/x", x)
	gitIn(t, dir, "update-ref", "refs/remotes/devvm/only/y", y)
	gitIn(t, dir, "symbolic-ref", "refs/remotes/devvm/HEAD", "refs/remotes/devvm/main")
	gitIn(t, dir, "config", "branch.main.remote", "devvm")
	gitIn(t, dir, "config", "branch.main.merge", "refs/heads/main")
	gitIn(t, dir, "branch", "saved/devvm/old")
	e.removeRemote(t.Context(), dir, vm, "devvm")
	if out := gitIn(t, dir, "remote"); out != "" {
		t.Errorf("remotes %q", out)
	}
	if out := gitIn(t, dir, "for-each-ref", "--format=%(objectname) %(refname)"); out != one+" refs/heads/main\n"+
		x+" refs/heads/saved/devvm-2/feature/x\n"+y+" refs/heads/saved/devvm-2/only/y\n"+one+" refs/heads/saved/devvm/old" {
		t.Errorf("refs:\n%s", out)
	}
	if out, err := hostGit(t.Context(), dir, "config", "--get-regexp", "^branch\\.main\\.remote"); err == nil {
		t.Errorf("upstream %q", out)
	}
	want := []string{`saved_branches branches="saved/devvm-2/feature/x" "saved/devvm-2/only/y"`, "removed_remote remote=devvm"}
	if !slices.Equal(*got, want) {
		t.Errorf("printed %q", *got)
	}

	// No free names: the remote and its branches stay.
	*got = nil
	gitIn(t, dir, "remote", "add", "devvm", cloneURL(vm, dir))
	gitIn(t, dir, "update-ref", "refs/remotes/devvm/z", gitIn(t, dir, "commit-tree", "-p", one, "-m", "z", tree))
	gitIn(t, dir, "branch", "-D", "saved/devvm/old", "saved/devvm-2/feature/x", "saved/devvm-2/only/y")
	gitIn(t, dir, "branch", "saved")
	e.removeRemote(t.Context(), dir, vm, "devvm")
	gitIn(t, dir, "remote", "get-url", "devvm")
	gitIn(t, dir, "rev-parse", "--verify", "refs/remotes/devvm/z")
	if !slices.Equal(*got, []string{"remote_kept remote=devvm"}) {
		t.Errorf("printed %q", *got)
	}
	e.removeRemote(t.Context(), dir, vm, "")
}
