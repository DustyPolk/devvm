package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootOfFollowsLinks(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	install := filepath.Join(tmp, "share", "devvm")
	bin := filepath.Join(tmp, "bin")
	for _, d := range []string{install, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(install, "devvm")
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "devvm")
	if err := os.Symlink(exe, link); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{exe, link} {
		got, err := rootOf(path)
		if err != nil {
			t.Fatalf("rootOf(%s): %v", path, err)
		}
		if got != install {
			t.Errorf("rootOf(%s) = %s, want %s", path, got, install)
		}
	}
}

func TestRootOfMissing(t *testing.T) {
	if _, err := rootOf(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Error("rootOf a missing file: want an error")
	}
}
