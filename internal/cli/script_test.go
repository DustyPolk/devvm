package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
			"wait_file":  waitFile,
		},
	})
}

// buildDevvm builds cmd/devvm into a temporary folder, with copies of the
// files devvm reads next to itself, and returns that folder.
func buildDevvm(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), "go", "install", "github.com/w3cj/devvm/cmd/devvm")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOBIN="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building devvm: %v\n%s", err, out)
	}
	repo := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(repo, "base.yaml")) //nolint:gosec // the repo's own file
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "base.yaml"), data, 0o600) //nolint:gosec // the test's own folder
	}
	for _, d := range []string{"layers", "tools", "guest"} {
		if err == nil {
			err = os.CopyFS(filepath.Join(dir, d), os.DirFS(filepath.Join(repo, d)))
		}
	}
	if err != nil {
		t.Fatal(err)
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
	// Ubuntu's cloud images: one dated release in a folder, never the
	// network.
	rel := filepath.Join(root, "ubuntu", "release-20260101")
	if err := os.MkdirAll(rel, 0o750); err != nil {
		return err
	}
	sums := strings.Repeat("1", 64) + " *ubuntu-26.04-server-cloudimg-arm64.img\n" +
		strings.Repeat("2", 64) + " *ubuntu-26.04-server-cloudimg-amd64.img\n"
	if err := os.WriteFile(filepath.Join(rel, "SHA256SUMS"), []byte(sums), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(rel, "SHA256SUMS.gpg"), []byte("signature\n"), 0o600); err != nil {
		return err
	}
	env.Setenv("DEVVM_UBUNTU_URL", "file://"+filepath.Join(root, "ubuntu"))
	env.Setenv("DEVVM_SECRET_SERVICE", "devvm-test")
	// The shared tools' versions: the stub mise, which resolves the
	// fixture lock.
	env.Setenv("DEVVM_HOST_MISE", filepath.Join(stubs, "mise"))
	env.Setenv("MISE_STUB_LOCK", filepath.Join(filepath.Dir(stubs), "mise.lock"))
	env.Setenv("DEVVM_BIN", bin) // for scripts that set their own PATH
	env.Setenv("PATH", strings.Join([]string{bin, stubs, env.Getenv("PATH")}, string(os.PathListSeparator)))
	return nil
}

// waitFile waits up to 10 seconds for a file to exist, for a background
// command to get somewhere.
func waitFile(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: wait_file FILE")
	}
	for range 200 {
		if _, err := os.Stat(ts.MkAbs(args[0])); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	ts.Fatalf("%s didn't appear", args[0])
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
