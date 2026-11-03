// Package stubtest is for tests: it puts the repo's stub commands
// (testdata/stubs) first on PATH, in place of the host's, with a new
// folder of their own for one test, and reads and writes what they keep
// there. The stubs find that folder in STUB_DIR and log each call they
// get to STUB_LOG, its calls file, one line each, written whole by
// stub-log so that stubs running at once never mix their lines.
package stubtest

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

// Stubs is one test's stubs: Dir is their folder, STUB_DIR.
type Stubs struct {
	Dir string
}

// New puts every stub first on PATH for t, or only the ones named, with
// a new folder under /tmp (short, for the sockets some stubs make in it)
// that t's end removes.
func New(t testing.TB, only ...string) *Stubs {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "stub.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		t.Fatal(err)
	}
	stubs, err := Folder()
	if err != nil {
		t.Fatal(err)
	}
	bin := stubs
	if len(only) > 0 {
		bin = filepath.Join(dir, "bin")
		if err := os.Mkdir(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		// stub-log too, which the stubs run from their own folder.
		for _, name := range append(slices.Clip(only), "stub-log") {
			stub := filepath.Join(stubs, name)
			if _, err := os.Stat(stub); err != nil {
				t.Fatalf("no stub %s: %v", name, err)
			}
			if err := os.Symlink(stub, filepath.Join(bin, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	s := &Stubs{Dir: dir}
	t.Setenv("STUB_DIR", dir)
	t.Setenv("STUB_LOG", s.Log())
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return s
}

// Folder returns the repo's stubs folder, testdata/stubs, found from the
// test's own folder up to the module's root.
func Folder() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "testdata", "stubs"), nil
		}
		up := filepath.Dir(dir)
		if up == dir {
			return "", errors.New("stubtest: no go.mod above the test's folder")
		}
		dir = up
	}
}

// Path returns the path of the file name in the stubs' folder.
func (s *Stubs) Path(name string) string { return filepath.Join(s.Dir, name) }

// Log returns the stubs' call log, STUB_LOG.
func (s *Stubs) Log() string { return s.Path("calls") }

// Calls returns the calls the stubs logged, one per line, in order: nil
// for none.
func (s *Stubs) Calls(t testing.TB) []string {
	t.Helper()
	text := strings.TrimSpace(s.Read(t, "calls"))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// Reset forgets the calls logged so far.
func (s *Stubs) Reset(t testing.TB) {
	t.Helper()
	if err := os.Remove(s.Log()); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

// Touch makes the empty file name in the stubs' folder, the switch a stub
// looks for.
func (s *Stubs) Touch(t testing.TB, name string) {
	t.Helper()
	s.Write(t, name, "")
}

// Write writes content to the file name in the stubs' folder, making the
// folders it's in.
func (s *Stubs) Write(t testing.TB, name, content string) {
	t.Helper()
	p := s.Path(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Read returns the file name in the stubs' folder: "" when there's none.
func (s *Stubs) Read(t testing.TB, name string) string {
	t.Helper()
	b, err := os.ReadFile(s.Path(name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(b)
}

// ExecRunner runs commands as they are, here the stubs: it's a backend's
// runner (Run, Output, Attach) with no run log or status line.
type ExecRunner struct{}

// Run runs cmd; step goes nowhere.
func (ExecRunner) Run(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

// Output runs cmd and returns its standard output.
func (ExecRunner) Output(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

// Attach runs cmd on the streams already set on it.
func (ExecRunner) Attach(cmd *exec.Cmd) error { return cmd.Run() }
