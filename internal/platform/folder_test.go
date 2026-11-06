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

// UniqueFolderID: an ID with a birth time of a folder on APFS or HFS+ on
// macOS, or on one of uniqueFS's file systems on Linux; never a missing
// folder.
func TestUniqueFolderID(t *testing.T) {
	dir := t.TempDir()
	id, err := FolderID(dir)
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "linux":
		// The temporary folder's file system may be any (TestUniqueFS).
		if UniqueFolderID(dir, "1:2") || UniqueFolderID(filepath.Join(dir, "missing"), "1:2:3.4") {
			t.Error("UniqueFolderID: an ID without a birth time, or a missing folder")
		}
		t.Logf("%s: %s, unique %v", dir, id, UniqueFolderID(dir, id))
	case "darwin":
		// The tests' temporary folder is on the system's APFS volume.
		if !UniqueFolderID(dir, id) || UniqueFolderID(filepath.Join(dir, "missing"), id) || UniqueFolderID(dir, "1:2") {
			t.Errorf("UniqueFolderID(%s) = %v", dir, UniqueFolderID(dir, id))
		}
	}
}

// VolPath names a FolderID's folder by its device and inode, and on macOS
// opens that folder once it's moved, never what replaced it at its path.
func TestVolPath(t *testing.T) {
	for id, want := range map[string]string{
		"": "", "12": "", "12:": "", ":34": "", "-1:34": "", "12:x": "", "1 2:34": "",
		"12:34": "/.vol/12/34", "12:34:5.6": "/.vol/12/34",
	} {
		if got := VolPath(id); got != want {
			t.Errorf("VolPath(%q) = %q, want %q", id, got, want)
		}
	}
	if runtime.GOOS != "darwin" {
		return
	}
	dir := filepath.Join(t.TempDir(), "p")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "own"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := FolderID(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(VolPath(id), "own")); err != nil {
		t.Errorf("%s, once its folder moved: %v", VolPath(id), err)
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
