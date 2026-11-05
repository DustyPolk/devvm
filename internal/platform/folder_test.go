//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"runtime"
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
	// A folder moved keeps its ID.
	if moved, _ := FolderID(dir + ".old"); moved != a {
		t.Errorf("a moved folder's ID: %q, was %q", moved, a)
	}
}

// UniqueFolderID: an ID with a birth time, on Linux, and on macOS of a
// folder on APFS or HFS+; never a missing folder on macOS.
func TestUniqueFolderID(t *testing.T) {
	dir := t.TempDir()
	id, err := FolderID(dir)
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "linux":
		if !UniqueFolderID(dir, "1:2:3.4") || UniqueFolderID(dir, "1:2") {
			t.Error("UniqueFolderID: by the birth time in the ID")
		}
		t.Logf("%s: %s, unique %v", dir, id, UniqueFolderID(dir, id))
	case "darwin":
		// The tests' temporary folder is on the system's APFS volume.
		if !UniqueFolderID(dir, id) || UniqueFolderID(filepath.Join(dir, "missing"), id) || UniqueFolderID(dir, "1:2") {
			t.Errorf("UniqueFolderID(%s) = %v", dir, UniqueFolderID(dir, id))
		}
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
