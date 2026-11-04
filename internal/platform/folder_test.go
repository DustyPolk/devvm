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

// DeviceOf gives the device FolderID records, which FolderDevice reads.
func TestDeviceOf(t *testing.T) {
	dir := t.TempDir()
	id, err := FolderID(dir)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := DeviceOf(dir)
	if err != nil || dev == "" || dev != FolderDevice(id) {
		t.Errorf("DeviceOf = %q, %v; FolderID = %q", dev, err, id)
	}
	if _, err := DeviceOf(filepath.Join(dir, "missing")); err == nil {
		t.Error("DeviceOf of a missing path: no error")
	}
	for id, want := range map[string]string{"": "", "12": "", "12:34": "12", "12:34:5.6": "12"} {
		if got := FolderDevice(id); got != want {
			t.Errorf("FolderDevice(%q) = %q, want %q", id, got, want)
		}
	}
}
