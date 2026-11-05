package vm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/stubtest"
	"github.com/w3cj/devvm/internal/vmssh"
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
	vmfs = stubtest.New(t, "ssh").Path("vmfs")
	f.Target = backend.SSHTarget{Host: "127.0.0.1", Port: 60022, User: "u", Key: "/k"}
	f.ExecFunc = func(_ string, argv []string, o backend.ExecOpts) error {
		var cmd *exec.Cmd
		switch {
		case len(argv) == 3 && strings.Contains(argv[2], "ssh_host_") && o.Stdout != nil:
			_, err := io.WriteString(o.Stdout, testHostKey+"\n")
			return err
		case len(argv) > 4 && argv[0] == "git" && argv[1] == "-C" && argv[3] == "worktree" && argv[4] == "list":
			// Its paths as the VM's, without vmfs.
			out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", vmfs + argv[2]}, argv[3:]...)...).Output() //nolint:gosec // the test's own
			if err != nil {
				return err
			}
			_, err = o.Stdout.Write(bytes.ReplaceAll(out, []byte("worktree "+vmfs), []byte("worktree ")))
			return err
		case len(argv) == 5 && argv[0] == "sh" && argv[2] == worktreeStatus:
			cmd = exec.CommandContext(t.Context(), "sh", "-c", argv[2], argv[3], vmfs+argv[4]) //nolint:gosec // the test's own
		case len(argv) > 3 && argv[0] == "git" && argv[1] == "-C":
			cmd = exec.CommandContext(t.Context(), "git", append([]string{"-C", vmfs + argv[2]}, argv[3:]...)...) //nolint:gosec // the test's own
		case len(argv) == 6 && argv[0] == "sh" && strings.Contains(argv[2], "receive.denyCurrentBranch"):
			cmd = exec.CommandContext(t.Context(), "bash", "-c", `sudo() { "$@"; }; `+argv[2], argv[3], vmfs+argv[4], argv[5]) //nolint:gosec // the test's own
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
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	vm := st.Name()
	if spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec); spec.Project != "" || spec.Tools == "" {
		t.Errorf("spec %+v", spec)
	}
	for k, want := range map[string]string{KeyMode: ModeClone, KeySeeded: "yes", KeyCloneRemote: "devvm", keyGitProtect: gitprotect.Clone} {
		if got := get(st, k); got != want {
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
	if err := (Launch{e}).Up(t.Context(), Options{Clone: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	// The ssh stub's folder is the one vmfs is in.
	calls := strings.Join((&stubtest.Stubs{Dir: filepath.Dir(vmfs)}).Calls(t), "\n")
	if n := strings.Count(calls, "git-receive-pack"); n != 1 {
		t.Errorf("pushed %d times:\n%s", n, calls)
	}
}

// The seed push goes to the VM itself, whatever host and path the repo's
// url.*.insteadOf rewrites the VM's URL to.
func TestUpCloneInsteadOf(t *testing.T) {
	e, _, vmfs := cloneEnv(t)
	gitIn(t, e.Dir, "config", "url.elsewhere.example:/x.insteadOf", "devvm-web-")
	existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	if got := gitIn(t, vmfs+e.Dir, "branch", "--format=%(refname:short)"); got != "main" {
		t.Errorf("clone's branches %q", got)
	}
	calls := strings.Join((&stubtest.Stubs{Dir: filepath.Dir(vmfs)}).Calls(t), "\n")
	if !strings.Contains(calls, " -p 60022 -l u 127.0.0.1 git-receive-pack '"+e.Dir+"'") || strings.Contains(calls, "elsewhere") {
		t.Errorf("ssh calls:\n%s", calls)
	}
}

// git's ssh for a --clone VM runs only receive-pack or upload-pack, on
// the clone, over ssh to the VM; anything else fails without ssh.
func TestGitSSHCommand(t *testing.T) {
	stubs := stubtest.New(t, "ssh")
	s := vmssh.SSH{VM: "devvm-web-0123456789abcd", User: "u", Key: "/k", KnownHosts: "/kh", Port: 60022}
	cmd := gitSSHCommand(s, "/p/it's")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-p", "22", "evil.example", "git-upload-pack '/elsewhere'"}, ` 127.0.0.1 git-upload-pack '/p/it'\''s'`},
		{[]string{"-o", "SendEnv=GIT_PROTOCOL", "evil.example", "git-receive-pack '/elsewhere'"}, ` 127.0.0.1 git-receive-pack '/p/it'\''s'`},
		{[]string{"evil.example", "touch /tmp/x"}, ""},
		{nil, ""},
	} {
		stubs.Reset(t)
		err := exec.CommandContext(t.Context(), "sh", append([]string{"-c", cmd + ` "$@"`, "ssh"}, c.args...)...).Run() //nolint:gosec // the test's own
		calls := strings.Join(stubs.Calls(t), "\n")
		if c.want == "" {
			if err == nil || calls != "" {
				t.Errorf("%q: ran, calls %q", c.args, calls)
			}
			continue
		}
		if !strings.HasPrefix(calls, "ssh -F none -T ") || !strings.HasSuffix(calls, c.want) || strings.Contains(calls, "evil") {
			t.Errorf("%q: calls %q", c.args, calls)
		}
	}
}

// A --clone VM of a linked worktree: its clone has only the worktree's
// branch and that branch's upstream, not the repo's other branches.
func TestUpCloneWorktree(t *testing.T) {
	e, _, vmfs := cloneEnv(t)
	repo := filepath.Join(filepath.Dir(e.Dir), "repo")
	if err := os.Rename(e.Dir, repo); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "branch", "other")
	gitIn(t, repo, "worktree", "add", "-q", "-b", "feature", e.Dir)
	gitIn(t, e.Dir, "branch", "-q", "--set-upstream-to=main")
	existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	clone := vmfs + e.Dir
	if got := gitIn(t, clone, "branch", "--format=%(refname:short)"); got != "feature\nmain" {
		t.Errorf("clone's branches %q", got)
	}
	if got := gitIn(t, clone, "symbolic-ref", "--short", "HEAD"); got != "feature" {
		t.Errorf("clone's HEAD %q", got)
	}
}

// rm on a --clone VM: branches with commits the repo lacks are offered
// for fetching, uncommitted changes in the clone are asked about, and
// the remote goes with the VM. A VM's answer that isn't an object name
// is never looked up.
func TestCloneRm(t *testing.T) {
	e, f, vmfs := cloneEnv(t)
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	vm, clone := st.Name(), vmfs+e.Dir
	gitIn(t, clone, "commit", "-q", "--allow-empty", "-m", "from-vm")

	// Not fetched, then dirty and kept.
	asked := answers(t, e, false)
	if err := os.WriteFile(filepath.Join(clone, "new"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
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
	// A remote named as the fetch's URL, whose own refspec would update
	// other branches, adds none.
	gitIn(t, e.Dir, "config", "remote."+cloneURL(vm, e.Dir)+".url", cloneURL(vm, e.Dir))
	gitIn(t, e.Dir, "config", "remote."+cloneURL(vm, e.Dir)+".fetch", "+refs/heads/*:refs/heads/planted/*")
	asked = answers(t, e, true)
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if out := gitIn(t, e.Dir, "for-each-ref", "--format=%(refname)", "refs/heads/planted"); out != "" {
		t.Errorf("the remote's refspec fetched %q", out)
	}
	gitIn(t, e.Dir, "remote", "remove", cloneURL(vm, e.Dir))
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

// rm's check of a stopped --clone VM starts it as a launch does: network
// policy's self-check stops one that gets out directly, and the VM is
// kept.
func TestCloneRmStartsAsLaunch(t *testing.T) {
	e, f, _ := cloneEnv(t)
	inner, direct := f.ExecFunc, false
	f.ExecFunc = func(vm string, argv []string, o backend.ExecOpts) error {
		if argv[0] == "curl" && !direct {
			return errors.New("curl: (7) Failed to connect")
		}
		return inner(vm, argv, o)
	}
	st := existingVM(t, e, Options{NetworkPolicy: true, Clone: true})
	vm := st.Name()
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	e.Proxy.Stop(vm)
	direct = true
	var warned msg.Recorder
	e.Warn = warned.Add
	before := len(f.Calls())
	err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true})
	// What kept it from starting is said, then the way out, carrying it.
	if msg.NamesOf(err) != "clone_check_not_started command=devvm rm vm="+vm {
		t.Errorf("gets out directly: %v", msg.NamesOf(err))
	}
	if !slices.Contains(warned.Names(), "net_direct_route vm="+vm) {
		t.Errorf("warned %q", warned.Names())
	}
	if msg.NamesOf(errors.Unwrap(err)) != "net_direct_route vm="+vm {
		t.Errorf("cause %v", errors.Unwrap(err))
	}
	// Nothing ran in it but the self-check, and the read of its host keys
	// for its port forwarder.
	for _, c := range f.Calls()[before:] {
		if c.Method != "Exec" {
			continue
		}
		if argv, _ := c.Args[1].([]string); len(argv) == 0 || argv[0] != "curl" && !slices.Equal(argv, []string{"sh", "-c", "cat /etc/ssh/ssh_host_*_key.pub"}) {
			t.Errorf("ran %q", argv)
		}
	}
	if in, ok := f.Instances[vm]; !ok || in.Status == backend.StatusRunning {
		t.Errorf("kept %v, %+v", ok, in)
	}
}

// rm on a --clone VM checks the clone's linked worktrees too: one with
// uncommitted changes is shown under its heading and asked about, one
// whose folder is gone is skipped, and the changes shown stop at
// cloneShown in all.
func TestCloneRmWorktrees(t *testing.T) {
	e, f, vmfs := cloneEnv(t)
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	vm, clone := st.Name(), vmfs+e.Dir
	gitIn(t, clone, "worktree", "add", "-q", ".worktrees/a")
	gitIn(t, clone, "worktree", "add", "-q", ".worktrees/gone")
	gitIn(t, clone, "worktree", "lock", ".worktrees/gone")
	if err := os.RemoveAll(filepath.Join(clone, ".worktrees", "gone")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, ".git", "info", "exclude"), []byte(".worktrees/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var said msg.Recorder
	e.Say = said.Add
	var out bytes.Buffer
	e.Stdout = &out

	// check runs rm, declining, and compares what it said and printed.
	check := func(want []string, wantOut string) {
		t.Helper()
		said.Reset()
		out = bytes.Buffer{}
		asked := answers(t, e, false)
		if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true}); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
		if !slices.Equal(said.Names(), want) || out.String() != wantOut || len(*asked) != min(len(want), 1) {
			t.Errorf("said %q, printed %q, asked %q", said.Names(), out.String(), *asked)
		}
	}
	// Dirty only in a: shown under its heading, at most cloneShown.
	for i := range cloneShown + 5 {
		if err := os.WriteFile(filepath.Join(clone, ".worktrees", "a", fmt.Sprintf("f%02d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var want strings.Builder
	for i := range cloneShown {
		fmt.Fprintf(&want, "?? f%02d\n", i)
	}
	check([]string{"clone_worktree_dirty path=" + e.Dir + "/.worktrees/a"}, want.String())

	// Dirty in the clone and a: each under its heading, cloneShown in all.
	if err := os.WriteFile(filepath.Join(clone, "new"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	want.Reset()
	want.WriteString("?? new\n")
	for i := range cloneShown - 1 {
		fmt.Fprintf(&want, "?? f%02d\n", i)
	}
	check([]string{"clone_dirty", "clone_worktree_dirty path=" + e.Dir + "/.worktrees/a"}, want.String())
	if _, ok := f.Instances[vm]; !ok {
		t.Fatal("deleted")
	}

	// Clean again, the gone one skipped: deleted.
	gitIn(t, clone, "worktree", "remove", "--force", ".worktrees/a")
	if err := os.Remove(filepath.Join(clone, "new")); err != nil {
		t.Fatal(err)
	}
	check(nil, "")
	if _, ok := f.Instances[vm]; ok {
		t.Error("kept")
	}
}

// The VM's worktree list is untrusted: only clean absolute paths below
// the clone are checked, each once, without bare or prunable ones or the
// clone itself, at most cloneWorktreesMax; a record cut off isn't used.
func TestCloneWorktrees(t *testing.T) {
	const dir = "/p/web"
	rec := func(lines ...string) string { return strings.Join(lines, "\x00") + "\x00\x00" }
	list := rec("worktree "+dir, "HEAD 0123", "branch refs/heads/main") +
		rec("worktree "+dir+"/.worktrees/a", "HEAD 0123", "branch refs/heads/a") +
		rec("worktree "+dir+"/.worktrees/a", "detached") +
		rec("worktree "+dir+"/.worktrees/new\nline", "detached", "locked reason\nwith a newline") +
		rec("worktree "+dir+"/.worktrees/b", "bare") +
		rec("worktree "+dir+"/.worktrees/c", "detached", "prunable gitdir file points to non-existent location") +
		rec("worktree "+dir+"/../etc", "detached") +
		rec("worktree "+dir+"//x", "detached") +
		rec("worktree "+dir+"2/x", "detached") +
		rec("worktree relative/x", "detached") +
		rec("worktree /etc", "detached") +
		rec("HEAD 0123") +
		"worktree " + dir + "/cut\x00detached"
	got, more := cloneWorktrees(dir, []byte(list))
	want := []string{dir + "/.worktrees/a", dir + "/.worktrees/new\nline"}
	if !slices.Equal(got, want) || more != 0 {
		t.Errorf("got %q, %d more", got, more)
	}

	var many strings.Builder
	for i := range cloneWorktreesMax + 3 {
		many.WriteString(rec(fmt.Sprintf("worktree %s/w%d", dir, i), "detached"))
	}
	if got, more := cloneWorktrees(dir, []byte(many.String())); len(got) != cloneWorktreesMax || more != 3 {
		t.Errorf("many: %d checked, %d more", len(got), more)
	}
}

// A clone with more worktrees than rm checks is asked about, even when
// none it checked has changes.
func TestCloneRmTooManyWorktrees(t *testing.T) {
	e, f, _ := cloneEnv(t)
	st := existingVM(t, e, Options{NoNetworkPolicy: true, Clone: true})
	vm := st.Name()
	// The VM lists more worktrees than devvm checks, none of them there.
	var list strings.Builder
	for i := range cloneWorktreesMax + 1 {
		fmt.Fprintf(&list, "worktree %s/w%d\x00detached\x00\x00", e.Dir, i)
	}
	inner := f.ExecFunc
	f.ExecFunc = func(name string, argv []string, o backend.ExecOpts) error {
		if slices.Contains(argv, "worktree") {
			_, err := o.Stdout.Write([]byte(list.String()))
			return err
		}
		return inner(name, argv, o)
	}
	var said msg.Recorder
	e.Say = said.Add
	asked := answers(t, e, false)
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if _, ok := f.Instances[vm]; !ok || !slices.Equal(said.Names(), []string{"clone_worktrees_unchecked max=100"}) || !slices.Equal(*asked, []string{"delete_anyway"}) {
		t.Errorf("said %q, asked %q", said.Names(), *asked)
	}
}

// Branch names git refuses are skipped, and never asked about.
func TestFetchUnfetchedInvalid(t *testing.T) {
	e, f, _ := cloneEnv(t)
	got := printed(t, e, true)
	check := cloneCheck{clones: clones{e}, command: "devvm clone off", remakes: true}
	if err := check.fetchUnfetched(t.Context(), f, "devvm-web-0123456789abcd", e.Dir, "devvm", []string{"a..b", "x y", "-x:refs/heads/main"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	want := []string{`branch_names_invalid refs="a..b" "x y" "-x:refs/heads/main"`}
	if !slices.Equal(*got, want) {
		t.Errorf("printed %q", *got)
	}
	gitIn(t, e.Dir, "branch", "saved")
	*got = nil
	err := check.fetchUnfetched(t.Context(), f, "devvm-web-0123456789abcd", e.Dir, "devvm", []string{"main"})
	if msg.NamesOf(err) != "saved_names_taken command=devvm clone off remote=devvm vm=devvm-web-0123456789abcd" || len(*got) > 0 {
		t.Errorf("no free names: %s, printed %q", msg.NamesOf(err), *got)
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
	check := func(d string) string { return msg.NamesOf(checkCloneRepo(t.Context(), d)) }
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

// --clone takes a repo of the folder's own, or a git worktree of one
// whose link git made both ways, and refuses a .git that points into
// another repo: a gitdir file to it, a worktree's gitdir file whose
// entry doesn't name this folder back, or alternates.
func TestCheckCloneRepoPointers(t *testing.T) {
	gitEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, wt := filepath.Join(root, "other"), filepath.Join(root, "wt")
	gitIn(t, root, "init", "-q", "-b", "main", other)
	gitIn(t, other, "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, other, "worktree", "add", "-q", "-b", "wt", wt)
	repo := func(name string) string {
		d := filepath.Join(root, name)
		gitIn(t, root, "init", "-q", "-b", "main", d)
		gitIn(t, d, "commit", "-q", "--allow-empty", "-m", "one")
		return d
	}
	gitFile := func(name, gitdir string) string {
		d := filepath.Join(root, name)
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: "+gitdir+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return d
	}
	alt := repo("alt")
	if err := os.WriteFile(filepath.Join(alt, ".git", "objects", "info", "alternates"), []byte(other+"/.git/objects\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pointed := gitFile("pointed", filepath.Join(other, ".git"))
	forged := gitFile("forged", filepath.Join(other, ".git", "worktrees", "wt"))
	for _, c := range []struct{ dir, want string }{
		{repo("plain"), ""},
		{wt, ""},
		{pointed, "clone_git_elsewhere path=" + pointed + "/.git"},
		{forged, "clone_git_elsewhere path=" + forged + "/.git"},
		{alt, "clone_alternates path=" + alt + "/.git/objects/info/alternates"},
	} {
		if got := msg.NamesOf(checkCloneRepo(t.Context(), c.dir)); got != c.want {
			t.Errorf("%s: %q, want %q", filepath.Base(c.dir), got, c.want)
		}
	}
}

// A linked worktree's seed push sends only its branch and that branch's
// upstream; a repo's, every branch.
func TestSeedRefs(t *testing.T) {
	gitEnv(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main, wt := filepath.Join(root, "main"), filepath.Join(root, "wt")
	gitIn(t, root, "init", "-q", "-b", "main", main)
	gitIn(t, main, "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, main, "worktree", "add", "-q", "-b", "wt", wt)
	for _, c := range []struct {
		dir      string
		worktree bool
		want     []string
	}{
		{main, false, []string{"--all"}},
		{wt, true, []string{"refs/heads/wt:refs/heads/wt"}},
	} {
		got, err := cloneSource(t.Context(), c.dir)
		if err != nil || got != c.worktree {
			t.Errorf("%s: worktree %v, %v", c.dir, got, err)
		}
		if refs := seedRefs(t.Context(), c.dir, filepath.Base(c.dir), got); !slices.Equal(refs, c.want) {
			t.Errorf("%s: %q", c.dir, refs)
		}
	}
	gitIn(t, wt, "branch", "-q", "--set-upstream-to=main")
	if refs := seedRefs(t.Context(), wt, "wt", true); !slices.Equal(refs, []string{"refs/heads/wt:refs/heads/wt", "refs/heads/main:refs/heads/main"}) {
		t.Errorf("with its upstream: %q", refs)
	}
	gitIn(t, wt, "config", "branch.wt.merge", "refs/heads/wt")
	if refs := seedRefs(t.Context(), wt, "wt", true); !slices.Equal(refs, []string{"refs/heads/wt:refs/heads/wt"}) {
		t.Errorf("its own upstream: %q", refs)
	}
	// An upstream that's a pattern would push every branch: not one.
	gitIn(t, wt, "config", "branch.wt.merge", "refs/heads/*")
	if refs := seedRefs(t.Context(), wt, "wt", true); !slices.Equal(refs, []string{"refs/heads/wt:refs/heads/wt"}) {
		t.Errorf("a pattern as its upstream: %q", refs)
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
			got = "error " + msg.NamesOf(err)
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
	clones{e}.removeRemote(t.Context(), dir, vm, "devvm")
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
	clones{e}.removeRemote(t.Context(), dir, vm, "devvm")
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
	clones{e}.removeRemote(t.Context(), dir, vm, "devvm")
	gitIn(t, dir, "remote", "get-url", "devvm")
	gitIn(t, dir, "rev-parse", "--verify", "refs/remotes/devvm/z")
	if !slices.Equal(*got, []string{"remote_kept remote=devvm"}) {
		t.Errorf("printed %q", *got)
	}
	clones{e}.removeRemote(t.Context(), dir, vm, "")
}
