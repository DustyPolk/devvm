//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

// A folder made again at the same path is another folder.
func TestFolderID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "p")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	a, err := FolderID(dir)
	if err != nil || a == "" {
		t.Fatalf("FolderID = %q, %v", a, err)
	}
	if again, _ := FolderID(dir); again != a {
		t.Errorf("FolderID changed: %q, then %q", a, again)
	}
	// Another folder kept alive, so its inode can't be the one reused.
	if err := os.Rename(dir, dir+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if b, _ := FolderID(dir); b == a {
		t.Errorf("a new folder at the same path has the old one's ID %q", a)
	}
	if _, err := FolderID(filepath.Join(dir, "missing")); err == nil {
		t.Error("FolderID of a missing folder: no error")
	}
}
