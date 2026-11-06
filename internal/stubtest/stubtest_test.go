package stubtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

// The stubs come first on PATH, log their calls to the sandbox's own
// folder, and read the files a test writes there; a reset forgets the
// calls.
func TestStubs(t *testing.T) {
	s := New(t)
	if !strings.HasPrefix(s.Dir, "/") || os.Getenv("STUB_DIR") != s.Dir || os.Getenv("STUB_LOG") != s.Log() {
		t.Fatalf("Dir %q, STUB_DIR %q, STUB_LOG %q", s.Dir, os.Getenv("STUB_DIR"), os.Getenv("STUB_LOG"))
	}
	if got, err := exec.LookPath("open"); err != nil || got != filepath.Join(Bin(t), "open") {
		t.Fatalf("open = %q, %v: not the stub", got, err)
	}
	if s.Calls(t) != nil {
		t.Errorf("calls before any: %q", s.Calls(t))
	}
	if err := exec.CommandContext(t.Context(), "open", "a").Run(); err != nil {
		t.Errorf("open a: %v", err)
	}
	s.Touch(t, "open-fails")
	if err := exec.CommandContext(t.Context(), "open", "b c").Run(); err == nil {
		t.Error("open with open-fails succeeded")
	}
	if got, want := s.Calls(t), []string{"open a", "open b c"}; !slices.Equal(got, want) {
		t.Errorf("Calls = %q, want %q", got, want)
	}
	s.Reset(t)
	s.Reset(t) // with nothing logged
	if s.Calls(t) != nil {
		t.Errorf("after Reset: %q", s.Calls(t))
	}
}

// Write makes the folders a file is in; Touch makes an empty one; Read
// gives "" for a missing one.
func TestWriteTouchRead(t *testing.T) {
	s := New(t)
	s.Write(t, "a/b/c", "x")
	if got := s.Read(t, "a/b/c"); got != "x" {
		t.Errorf("Read = %q", got)
	}
	s.Touch(t, "flag")
	if fi, err := os.Stat(s.Path("flag")); err != nil || fi.Size() != 0 {
		t.Errorf("Touch: %v, %v", fi, err)
	}
	if got := s.Read(t, "missing"); got != "" {
		t.Errorf("Read missing = %q", got)
	}
}

// A stub's long call lands whole in the call log while others write to
// it: two stubs at once, and this test appending short lines of its own
// as fast as it can, so a call written in pieces has another's line
// between them.
func TestCallLinesWhole(t *testing.T) {
	s := New(t)
	const runs = 20
	args := []string{strings.Repeat("a", 100000), strings.Repeat("b", 100000)}
	done := make(chan struct{})
	hammered := make(chan error, 1)
	go func() { hammered <- hammer(s.Log(), done) }()
	var wg sync.WaitGroup
	errs := make(chan error, len(args)*runs)
	for _, arg := range args {
		wg.Go(func() {
			for range runs {
				errs <- exec.CommandContext(t.Context(), "open", arg).Run() //nolint:gosec // the open stub, with the test's own argument
			}
		})
	}
	wg.Wait()
	close(done)
	close(errs)
	if err := <-hammered; err != nil {
		t.Fatal(err)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("open: %v", err)
		}
	}
	n := 0
	for _, c := range s.Calls(t) {
		switch c {
		case "x":
		case "open " + args[0], "open " + args[1]:
			n++
		default:
			t.Fatalf("a call's line is mixed with another's: %.60q…", c)
		}
	}
	if n != len(args)*runs {
		t.Errorf("%d calls logged whole, want %d", n, len(args)*runs)
	}
}

// A call with a start's limactl edit words, an ssh option and a proxy URL
// with its password, is logged whole: an endpoint security tool kills a
// perl with such words on its command line, so stub-log keeps them off it.
func TestCallLoggedWithEditWords(t *testing.T) {
	s := New(t)
	proxy := "http://devvm:devvm-proxy-password@192.168.5.2:3128" //nolint:gosec // devvm's stand-in, not a credential
	args := []string{
		"edit", "--set", ".ssh.forwardAgent=false", "--set",
		`.env = {"HTTPS_PROXY":"` + proxy + `","HTTP_PROXY":"` + proxy + `","NODE_EXTRA_CA_CERTS":"/etc/ssl/devvm.pem",` +
			`"NO_PROXY":"localhost,127.0.0.1,::1","http_proxy":"` + proxy + `","https_proxy":"` + proxy + `"}`,
	}
	if out, err := exec.CommandContext(t.Context(), "open", args...).CombinedOutput(); err != nil {
		t.Fatalf("open: %v\n%s", err, out)
	}
	if got, want := s.Calls(t), []string{"open " + strings.Join(args, " ")}; !slices.Equal(got, want) {
		t.Errorf("Calls = %q, want %q", got, want)
	}
}

// hammer appends the line "x" to log, one write each, until done closes.
func hammer(log string, done <-chan struct{}) error {
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600) //nolint:gosec // the test's own call log
	if err != nil {
		return err
	}
	for {
		select {
		case <-done:
			return f.Close()
		default:
		}
		if _, err := f.WriteString("x\n"); err != nil {
			_ = f.Close()
			return err
		}
	}
}

// Lima's instances, made by limactl processes running at once, are all
// kept: the state is changed under its lock.
func TestCreatesAtOnce(t *testing.T) {
	s := New(t)
	t.Setenv("LIMA_HOME", filepath.Join(s.Dir, "lima"))
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			errs <- exec.CommandContext(t.Context(), "limactl", "create", "--name=vm"+strconv.Itoa(i), "--tty=false", "-").Run() //nolint:gosec // the limactl stub, with the test's own name
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := s.VMs(t); len(got) != n {
		t.Errorf("%d instances, want %d: %q", len(got), n, got)
	}
}

// The binary builds into a folder with a link for each stub; its folder's
// name changes with its sources; a folder a day old is removed, but the
// one in use.
func TestBuild(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := buildInto(root, dir); err != nil {
		t.Fatal(err)
	}
	if l, err := os.Readlink(filepath.Join(dir, "limactl")); err != nil || l != "stub" {
		t.Errorf("limactl links to %q, %v", l, err)
	}
	sum, err := sourcesHash(root)
	if err != nil || len(sum) != 16 {
		t.Errorf("sourcesHash = %q, %v", sum, err)
	}
	if other, _ := sourcesHash(t.TempDir()); other == sum {
		t.Error("the hash doesn't change with the sources")
	}
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	old, keep, fresh := filepath.Join(tmp, "devvm-stubs-old"), filepath.Join(tmp, "devvm-stubs-keep"), filepath.Join(tmp, "devvm-stubs-new")
	day := time.Now().Add(-25 * time.Hour)
	for _, d := range []string{old, keep, fresh} {
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{old, keep} {
		if err := os.Chtimes(d, day, day); err != nil {
			t.Fatal(err)
		}
	}
	prune(keep)
	if exists(old) || !exists(keep) || !exists(fresh) {
		t.Errorf("after prune: old %v, keep %v, new %v", exists(old), exists(keep), exists(fresh))
	}
	t.Chdir(t.TempDir())
	if _, err := moduleRoot(); err == nil {
		t.Error("a module root outside the repo")
	}
}

// The state's helpers: an instance listed, its size, the Keychain.
func TestState(t *testing.T) {
	s := New(t)
	t.Setenv("LIMA_HOME", filepath.Join(s.Dir, "lima"))
	s.VM(t, "b", "Running")
	s.VM(t, "a", "Stopped")
	s.Size(t, "a", 2, 3, 4)
	s.VM(t, "a", "Running")
	if got, want := s.VMs(t), []string{"a Running", "b Running"}; !slices.Equal(got, want) {
		t.Errorf("VMs = %q, want %q", got, want)
	}
	out, err := exec.CommandContext(t.Context(), "limactl", "list", "--json").Output()
	if err != nil || !strings.HasPrefix(string(out), `{"name":"a","status":"Running",`) || !strings.Contains(string(out), `"cpus":2,"memory":3,"disk":4,`) {
		t.Errorf("list: %v\n%s", err, out)
	}
	// A broken instance's pids, listed; its driver left running holds
	// its disk, so start fails until a stop.
	s.VM(t, "b", "Broken")
	s.PIDs(t, "b", 0, 7)
	s.PIDs(t, "none", 1, 2)
	out, err = exec.CommandContext(t.Context(), "limactl", "list", "--json").Output()
	if err != nil || !strings.Contains(string(out), `"status":"Broken",`) || !strings.Contains(string(out), `"driverPID":7,`) {
		t.Errorf("list, broken: %v\n%s", err, out)
	}
	if exec.CommandContext(t.Context(), "limactl", "start", "--tty=false", "b").Run() == nil {
		t.Error("started over a driver left running")
	}
	if err := exec.CommandContext(t.Context(), "limactl", "stop", "--force", "b").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.CommandContext(t.Context(), "limactl", "start", "--tty=false", "b").Run(); err != nil {
		t.Errorf("start after a stop: %v", err)
	}
	if _, ok := s.Keychain(t, "github"); ok {
		t.Error("an item before any")
	}
	s.SetKeychain(t, "github", "v")
	if v, ok := s.Keychain(t, "github"); !ok || v != "v" {
		t.Errorf("Keychain = %q, %v", v, ok)
	}
}

// With names, only those stubs are on PATH: the host's others stay.
func TestOnly(t *testing.T) {
	host, err := exec.LookPath("uname")
	if err != nil {
		t.Skip("no uname here")
	}
	s := New(t, "open")
	if got, _ := exec.LookPath("open"); got != filepath.Join(s.Dir, "bin", "open") {
		t.Errorf("open = %q", got)
	}
	if got, _ := exec.LookPath("uname"); got != host {
		t.Errorf("uname = %q, want the host's %q", got, host)
	}
}

// ExecRunner runs commands as they are.
func TestExecRunner(t *testing.T) {
	var r ExecRunner
	if err := r.Run(t.Context(), msg.HomeNotAbsolute(), exec.CommandContext(t.Context(), "true")); err != nil {
		t.Errorf("Run: %v", err)
	}
	if out, err := r.Output(exec.CommandContext(t.Context(), "echo", "hi")); err != nil || string(out) != "hi\n" {
		t.Errorf("Output = %q, %v", out, err)
	}
	if err := r.Attach(exec.CommandContext(t.Context(), "false")); err == nil {
		t.Error("Attach of false succeeded")
	}
}
