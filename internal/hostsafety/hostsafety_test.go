package hostsafety

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func touch(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetIDFiles(t *testing.T) {
	d := t.TempDir()
	mkdirs(t, filepath.Join(d, "sub"))
	tool := filepath.Join(d, "sub", "tool")
	touch(t, filepath.Join(d, "plain"), tool)
	if err := os.Chmod(tool, fs.ModeSetuid|0o755); err != nil {
		t.Fatal(err)
	}
	if got := SetIDFiles(d); !slices.Equal(got, []SetIDFile{{Path: tool, Root: os.Getuid() == 0}}) {
		t.Errorf("SetIDFiles = %v, want %s", got, tool)
	}
	if err := os.Chmod(tool, fs.ModeSetgid|0o755); err != nil {
		t.Fatal(err)
	}
	if got := SetIDFiles(d); len(got) != 1 {
		t.Errorf("set-group-id: SetIDFiles = %v", got)
	}
	if err := os.Chmod(tool, 0o755); err != nil { //nolint:gosec // the test's own file
		t.Fatal(err)
	}
	if got := SetIDFiles(d); got != nil {
		t.Errorf("none: SetIDFiles = %v", got)
	}
	// None in a .git folder, the project's or a submodule's, counts.
	for _, g := range []string{filepath.Join(d, ".git", "hooks"), filepath.Join(d, "sub", ".git")} {
		mkdirs(t, g)
		p := filepath.Join(g, "hook")
		touch(t, p)
		if err := os.Chmod(p, fs.ModeSetuid|0o700); err != nil {
			t.Fatal(err)
		}
	}
	if got := SetIDFiles(d); got != nil {
		t.Errorf("in .git: SetIDFiles = %v", got)
	}
	// At most five.
	for i := range 7 {
		p := filepath.Join(d, "s"+string(rune('a'+i)))
		touch(t, p)
		if err := os.Chmod(p, fs.ModeSetuid|0o700); err != nil {
			t.Fatal(err)
		}
	}
	if got := SetIDFiles(d); len(got) != setIDMax {
		t.Errorf("SetIDFiles = %d files, want %d", len(got), setIDMax)
	}
	if got := SetIDFiles(filepath.Join(d, "none")); got != nil {
		t.Errorf("no folder: SetIDFiles = %v", got)
	}
}

// Root's set-user-id files and root's group's set-group-id ones run as
// root; any other's, or root's without that bit, don't.
func TestRootSetID(t *testing.T) {
	for _, c := range []struct {
		name     string
		m        fs.FileMode
		uid, gid uint32
		want     bool
	}{
		{"set-user-id root's", fs.ModeSetuid | 0o755, 0, 1000, true},
		{"set-group-id root's group", fs.ModeSetgid | 0o755, 1000, 0, true},
		{"set-user-id the user's", fs.ModeSetuid | 0o755, 1000, 1000, false},
		{"set-group-id root's, the user's group", fs.ModeSetgid | 0o755, 0, 1000, false},
		{"set-user-id root's group", fs.ModeSetuid | 0o755, 1000, 0, false},
	} {
		if got := rootSetID(c.m, c.uid, c.gid); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}

func TestSSHRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	d := t.TempDir()
	if SSHRemote(t.Context(), d) {
		t.Error("no repo: SSHRemote")
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", d}, args...)...) //nolint:gosec // the test's own
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("remote", "add", "origin", "https://example.com/r")
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"git@github.com:o/r.git", true},
		{"ssh://git@example.com/r", true},
		{"https://github.com/o/r", false},
	} {
		git("remote", "set-url", "origin", tc.url)
		if got := SSHRemote(t.Context(), d); got != tc.want {
			t.Errorf("SSHRemote with %s = %v", tc.url, got)
		}
	}
}
