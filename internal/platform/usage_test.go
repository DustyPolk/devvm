package platform

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskUsage(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.WriteFile(a, make([]byte, 64<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	one, err := DiskUsage(dir)
	if err != nil || one < 64<<10 {
		t.Fatalf("DiskUsage = %d, %v; want at least 64 KiB", one, err)
	}
	// A second link to it, and a symlink, take nothing more.
	if err := os.Link(a, filepath.Join(dir, "b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/", filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
	two, err := DiskUsage(dir)
	if err != nil || two > one+64<<10-1 {
		t.Errorf("with a hard link and a symlink: DiskUsage = %d, %v; want about %d", two, err, one)
	}
	if _, err := DiskUsage(filepath.Join(dir, "nope")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing path: err = %v, want fs.ErrNotExist", err)
	}
}
