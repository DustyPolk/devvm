package stubtest

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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
