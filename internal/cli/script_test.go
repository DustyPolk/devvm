package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestScript runs testdata/script/*.txtar against a built devvm, in a sandbox.
func TestScript(t *testing.T) {
	bin := buildDevvm(t)
	stubs, err := filepath.Abs(filepath.Join("..", "..", "testdata", "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("..", "..", "testdata", "script"),
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Setup: func(env *testscript.Env) error {
			return sandbox(env, bin, stubs)
		},
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"stub_calls": stubCalls,
		},
	})
}

// buildDevvm builds cmd/devvm into a temporary folder and returns that folder.
func buildDevvm(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "go", "install", "github.com/w3cj/devvm/cmd/devvm")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOBIN="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building devvm: %v\n%s", err, out)
	}
	return dir
}

// sandbox points every folder devvm uses into a new temporary folder, and puts
// devvm and the stubs first on PATH.
func sandbox(env *testscript.Env, bin, stubs string) error {
	// /tmp keeps Lima's socket paths short.
	root, err := os.MkdirTemp("/tmp", "dvt.")
	if err != nil {
		return err
	}
	env.Defer(func() { _ = os.RemoveAll(root) })

	dirs := map[string]string{
		"HOME":            "home",
		"XDG_DATA_HOME":   "data",
		"XDG_CONFIG_HOME": "config",
		"DEVVM_LIMA_HOME": "lima",
		"LIMA_HOME":       "lima",
		"STUB_DIR":        "stub",
	}
	for name, dir := range dirs {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		env.Setenv(name, path)
	}
	log := filepath.Join(root, "stub", "calls")
	if err := os.WriteFile(log, nil, 0o600); err != nil {
		return err
	}
	env.Setenv("STUB_LOG", log)
	env.Setenv("DEVVM_SECRET_SERVICE", "devvm-test")
	env.Setenv("DEVVM_BIN", bin) // for scripts that set their own PATH
	env.Setenv("PATH", strings.Join([]string{bin, stubs, env.Getenv("PATH")}, string(os.PathListSeparator)))
	return nil
}

// stubCalls writes the calls the stubs logged to stdout, one per line.
// Negated, it fails if any were logged.
func stubCalls(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) > 0 {
		ts.Fatalf("usage: stub_calls")
	}
	calls := ts.ReadFile(ts.Getenv("STUB_LOG"))
	if neg {
		if calls != "" {
			ts.Fatalf("unexpected stub calls:\n%s", calls)
		}
		return
	}
	_, err := ts.Stdout().Write([]byte(calls))
	ts.Check(err)
}
