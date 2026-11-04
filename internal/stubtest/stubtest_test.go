package stubtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

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
	stubs, err := Folder()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := exec.LookPath("open"); err != nil || got != filepath.Join(stubs, "open") {
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

// Every stub logs its calls through stub-log, never with the shell's
// own append, which can write a long line in pieces.
func TestStubsLogWithStubLog(t *testing.T) {
	stubs, err := Folder()
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(stubs)
	if err != nil {
		t.Fatal(err)
	}
	appends := regexp.MustCompile(`>>\s*"?\$\{?(STUB_LOG|STUB_DIR)`)
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(stubs, f.Name())) //nolint:gosec // the repo's own stubs
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if appends.MatchString(line) {
				t.Errorf("%s:%d appends with the shell, not stub-log: %s", f.Name(), i+1, strings.TrimSpace(line))
			}
		}
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
