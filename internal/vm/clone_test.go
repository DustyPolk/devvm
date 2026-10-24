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
	asked := answers(t, false)
	if err := os.WriteFile(filepath.Join(clone, "new"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	want := []string{"fetch_before_delete remote=devvm", "delete_anyway"}
	if _, ok := f.Instances[vm]; !ok || !slices.Equal(*asked, want) {
		t.Fatalf("declined: asked %q", *asked)
	}
	if _, err := hostGit(t.Context(), e.Dir, "rev-parse", "-q", "--verify", "refs/remotes/devvm/main"); err == nil {
		t.Error("fetched without a yes")
	}

	// Stopped: started for the check. Fetched, then deleted.
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(clone, "new")); err != nil {
		t.Fatal(err)
	}
	asked = answers(t, true)
	if err := e.Rm(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	if _, ok := f.Instances[vm]; ok || !slices.Equal(*asked, want[:1]) {
		t.Errorf("confirmed: deleted %v, asked %q", !ok, *asked)
	}
	if got := gitIn(t, e.Dir, "log", "--format=%s", "-1", "refs/remotes/devvm/main"); got != "from-vm" {
		t.Errorf("fetched %q", got)
	}
	if out, err := hostGit(t.Context(), e.Dir, "remote"); err != nil || strings.Contains(out, "devvm") {
		t.Errorf("remotes %q", out)
	}

	refs := []byte("0123456789abcdef0123456789abcdef01234567 known\n$(touch-x) evil\nnot-hex\n\n" +
		gitIn(t, e.Dir, "rev-parse", "HEAD") + " main\n")
	got := unfetchedRefs(t.Context(), e.Dir, refs)
	if !slices.Equal(got, []string{"known", "evil", ""}) {
		t.Errorf("unfetched %q", got)
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

// The remote goes only while it's still the VM's.
func TestRemoveRemote(t *testing.T) {
	gitEnv(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "remote", "add", "devvm", "https://example.com/mine")
	removeRemote(t.Context(), dir, "devvm-web-0123456789abcd", "devvm")
	gitIn(t, dir, "remote", "get-url", "devvm")
	gitIn(t, dir, "remote", "set-url", "devvm", cloneURL("devvm-web-0123456789abcd", dir))
	removeRemote(t.Context(), dir, "devvm-web-0123456789abcd", "devvm")
	if out := gitIn(t, dir, "remote"); out != "" {
		t.Errorf("remotes %q", out)
	}
	removeRemote(t.Context(), dir, "devvm-web-0123456789abcd", "")
}
