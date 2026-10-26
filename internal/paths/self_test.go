package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFollowsLinks(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	install := filepath.Join(tmp, "share", "devvm")
	bin := filepath.Join(tmp, "bin")
	for _, d := range []string{install, bin} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(install, "devvm")
	if err := os.WriteFile(exe, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "devvm")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{exe, link} {
		got, err := resolve(path)
		if err != nil {
			t.Fatalf("resolve(%s): %v", path, err)
		}
		if got != exe {
			t.Errorf("resolve(%s) = %s, want %s", path, got, exe)
		}
	}
}

func TestResolveMissing(t *testing.T) {
	if _, err := resolve(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("resolve a missing file: want an error")
	}
}
