package project

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestSlug(t *testing.T) {
	for _, tc := range []struct{ base, want string }{
		{"My_App.v2", "my-app-v2"},
		{"My App", "my-app"},
		{"--web--", "web"},
		{"café", "caf"},
		{"___", "root"},
		{"/", "root"},
		{"a-very-long-folder-name-for-a-project", "a-very-long-folder-n"},
		{"a-very-long-folder-----x", "a-very-long-folder"}, // cut, then no trailing dash
	} {
		if got := Slug(tc.base, slugMax); got != tc.want {
			t.Errorf("Slug(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestIsVM(t *testing.T) {
	for name, want := range map[string]bool{
		"devvm-web-12345612345612":     true,
		"devvm-img-foo-abc123abc123ab": true, // a folder named img-foo
		"devvm-img-0123456789ab":       false,
		"devvm-tools-build":            false,
		"devvm-web-123456":             false,
		"other-web-12345612345612":     false,
	} {
		if got := IsVM(name); got != want {
			t.Errorf("IsVM(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	name := regexp.MustCompile(`^devvm-my-app-v2-[0-9a-f]{14}$`)
	f, err := New("/w/My_App.v2", 0)
	if err != nil || !name.MatchString(f.Name) || f.Dir != "/w/My_App.v2" {
		t.Errorf("New = %+v, %v", f, err)
	}
	if f.Slug() != "my-app-v2" {
		t.Errorf("Slug = %q", f.Slug())
	}
	// Another folder with the same name gets another VM.
	if g, _ := New("/x/My_App.v2", 0); g.Name == f.Name {
		t.Errorf("two folders share %s", f.Name)
	}
	// Spaces are fine; Lima can't mount commas or control characters.
	if f, err := New("/w/My App", 0); err != nil || f.Name[:13] != "devvm-my-app-" {
		t.Errorf("New with a space = %+v, %v", f, err)
	}
	for _, dir := range []string{"/w/a,b", "/w/a\tb", "/w/a\nb", "/w/a\x1b[2Jb"} {
		var bad *BadPathError
		if _, err := New(dir, 0); !errors.As(err, &bad) || bad.Dir != dir {
			t.Errorf("New(%q) = %v, want a BadPathError", dir, err)
		}
	}
}

// A long folder name is cut to fit the backend's names; names with no
// room for four characters of it are refused.
func TestNewFitsNameMax(t *testing.T) {
	dir := "/w/a-very-long-folder-name-for-a-project"
	f, err := New(dir, 0)
	if err != nil || !regexp.MustCompile(`^devvm-a-very-long-folder-n-[0-9a-f]{14}$`).MatchString(f.Name) {
		t.Errorf("New = %+v, %v", f, err)
	}
	f, err = New(dir, 30)
	if err != nil || len(f.Name) != 30 || f.Name[:15] != "devvm-a-very-lo" {
		t.Errorf("New with 30 = %+v, %v", f, err)
	}
	if _, err := New(dir, 6+4+1+14); err != nil {
		t.Errorf("New with room for four: %v", err)
	}
	if _, err := New(dir, 6+3+1+14); !errors.Is(err, ErrNoRoom) {
		t.Errorf("New with room for three = %v, want ErrNoRoom", err)
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

// A folder that is or holds the home folder or devvm's state, or is
// inside devvm's own folders, is unsafe; a project in the home folder isn't.
func TestUnsafe(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	u := Unsafe{
		Home:   filepath.Join(root, "home"),
		Config: filepath.Join(root, "config", "devvm"),
		Data:   filepath.Join(root, "data", "devvm"),
		Lima:   filepath.Join(root, "data", "devvm", "lima"),
	}
	mkdirs(t, u.Home, u.Config, u.Lima)
	for _, tc := range []struct{ dir, holds, inside string }{
		{u.Home, u.Home, ""},
		{root, u.Home, ""},
		{"/", u.Home, ""},
		{filepath.Join(root, "data"), u.Data, ""},
		{filepath.Join(root, "config"), u.Config, ""},
		{filepath.Join(u.Config, "layers"), "", u.Config},
		{filepath.Join(u.Data, "bin"), "", u.Data},
		{filepath.Join(u.Lima, "x"), "", u.Data},
		{filepath.Join(u.Home, "proj"), "", ""},
		{u.Home + "-sibling", "", ""},
	} {
		if got := u.Holds(tc.dir); got != tc.holds {
			t.Errorf("Holds(%s) = %q, want %q", tc.dir, got, tc.holds)
		}
		if got := u.Inside(tc.dir); got != tc.inside {
			t.Errorf("Inside(%s) = %q, want %q", tc.dir, got, tc.inside)
		}
	}
	// Through a symlink, as given: the folders are compared by their real paths.
	link := filepath.Join(root, "link")
	if err := os.Symlink(u.Home, link); err != nil {
		t.Fatal(err)
	}
	if got := (Unsafe{Home: link}).Holds(u.Home); got != link {
		t.Errorf("Holds through a link = %q", got)
	}
}

// Parents lists the folders a folder is inside, nearest first, stopping
// before the home folder (given through a symlink too) and the root.
func TestParents(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	mkdirs(t, home)
	link := filepath.Join(root, "link")
	if err := os.Symlink(home, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		dir, home string
		want      []string
	}{
		{filepath.Join(home, "app", "src", "lib"), home, []string{filepath.Join(home, "app", "src"), filepath.Join(home, "app")}},
		{filepath.Join(home, "app", "src"), link, []string{filepath.Join(home, "app")}},
		{filepath.Join(home, "app"), home, nil},
		{home, home, nil},
		{"/srv/app/src", home, []string{"/srv/app", "/srv"}},
		{"/srv", home, nil},
		{"/", home, nil},
	} {
		if got := Parents(tc.dir, tc.home); !slices.Equal(got, tc.want) {
			t.Errorf("Parents(%s, %s) = %q, want %q", tc.dir, tc.home, got, tc.want)
		}
	}
}

func touch(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetect(t *testing.T) {
	d := t.TempDir()
	if got := Detect(d); got != nil {
		t.Errorf("Detect(empty) = %v", got)
	}
	for _, tc := range []struct{ file, want string }{
		{"package.json", "node"},
		{"bun.lockb", "node"},
		{"uv.lock", "python"},
		{"requirements.txt", "python"},
		{"Pipfile", "python"},
		{"Cargo.toml", "rust"},
		{"go.mod", "go"},
		{"flake.nix", "nix"},
		{"shell.nix", "nix"},
		{"devenv.nix", "nix"},
	} {
		p := filepath.Join(d, tc.file)
		touch(t, p)
		if got := Detect(d); !slices.Equal(got, []string{tc.want}) {
			t.Errorf("Detect with %s = %v, want %s", tc.file, got, tc.want)
		}
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	// Several, in a fixed order; only the top level counts.
	mkdirs(t, filepath.Join(d, "sub"))
	touch(t, filepath.Join(d, "go.mod"), filepath.Join(d, "package.json"), filepath.Join(d, "pyproject.toml"),
		filepath.Join(d, "sub", "Cargo.toml"))
	if got := Detect(d); !slices.Equal(got, []string{"node", "python", "go"}) {
		t.Errorf("Detect = %v", got)
	}
	touch(t, filepath.Join(d, "flake.nix"))
	if got := Detect(d); !slices.Equal(got, []string{"node", "python", "go", "nix"}) {
		t.Errorf("Detect with flake.nix = %v", got)
	}
}

// A default.nix calls for nix only when it makes a shell, and only as a
// regular file.
func TestDetectDefaultNix(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "default.nix")
	for _, tc := range []struct {
		body string
		want []string
	}{
		{"{ pkgs ? import <nixpkgs> {} }: pkgs.mkShell { packages = [ pkgs.hello ]; }", []string{"nix"}},
		{"{ pkgs ? import <nixpkgs> {} }: pkgs.mkShellNoCC { }", []string{"nix"}},
		{"{ pkgs ? import <nixpkgs> {} }: pkgs.callPackage ./package.nix { }", nil},
	} {
		if err := os.WriteFile(p, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := Detect(d); !slices.Equal(got, tc.want) {
			t.Errorf("Detect with default.nix %q = %v, want %v", tc.body, got, tc.want)
		}
	}
	shell := filepath.Join(t.TempDir(), "shell.nix")
	if err := os.WriteFile(shell, []byte("pkgs.mkShell { }"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shell, p); err != nil {
		t.Fatal(err)
	}
	if got := Detect(d); got != nil {
		t.Errorf("Detect with a linked default.nix = %v", got)
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
