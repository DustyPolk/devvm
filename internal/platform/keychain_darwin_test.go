package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubs puts the test stubs first on PATH, never the real security or
// tmutil, and returns their folder and their call log.
func stubs(t *testing.T) (dir, log string) {
	t.Helper()
	bin, err := filepath.Abs(filepath.Join("..", "..", "testdata", "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	dir = t.TempDir()
	log = filepath.Join(dir, "calls")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("STUB_DIR", dir)
	t.Setenv("STUB_LOG", log)
	return dir, log
}

func TestKeychainGet(t *testing.T) {
	dir, log := stubs(t)
	if _, err := KeychainGet(t.Context(), "devvm-test", "github"); !errors.Is(err, ErrNoSecret) {
		t.Errorf("no item: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "keychain"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keychain", "github"), []byte("ghp_x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if v, err := KeychainGet(t.Context(), "devvm-test", "github"); v != "ghp_x" || err != nil {
		t.Errorf("KeychainGet = %q, %v", v, err)
	}
	calls, _ := os.ReadFile(log) //nolint:gosec // the test's own folder
	if !strings.HasSuffix(string(calls), "security find-generic-password -s devvm-test -a github -w\n") {
		t.Errorf("calls = %q", calls)
	}
}

// A value is saved through security -i's stdin, never in its arguments;
// one that would end that command line is refused. Deleting a missing
// item is ErrNoSecret.
func TestKeychainSetDelete(t *testing.T) {
	_, log := stubs(t)
	ctx := t.Context()
	if err := KeychainSet(ctx, "devvm-test", "github", "devvm github", "ghp_testfake"); err != nil {
		t.Fatal(err)
	}
	if v, err := KeychainGet(ctx, "devvm-test", "github"); v != "ghp_testfake" || err != nil {
		t.Errorf("KeychainGet = %q, %v", v, err)
	}
	for _, bad := range []string{`x" -A "`, `x\`, "x\ny"} {
		if err := KeychainSet(ctx, "devvm-test", "github", "devvm github", bad); err == nil {
			t.Errorf("KeychainSet(%q) took it", bad)
		}
	}
	if err := KeychainDelete(ctx, "devvm-test", "github"); err != nil {
		t.Errorf("KeychainDelete: %v", err)
	}
	if err := KeychainDelete(ctx, "devvm-test", "github"); !errors.Is(err, ErrNoSecret) {
		t.Errorf("KeychainDelete again: %v", err)
	}
	calls, _ := os.ReadFile(log) //nolint:gosec // the test's own folder
	if strings.Contains(string(calls), "ghp_testfake") || !strings.HasPrefix(string(calls), "security -i\n") {
		t.Errorf("calls = %q", calls)
	}
}

// The CA folder is kept out of Time Machine, also one made before devvm
// did that: only when it isn't already.
func TestExcludeFromBackup(t *testing.T) {
	_, log := stubs(t)
	for range 2 {
		if err := ExcludeFromBackup(t.Context(), "/data/ca"); err != nil {
			t.Fatal(err)
		}
	}
	if calls, _ := os.ReadFile(log); string(calls) != "tmutil addexclusion /data/ca\n" { //nolint:gosec // the test's own folder
		t.Errorf("calls = %q", calls)
	}
}
