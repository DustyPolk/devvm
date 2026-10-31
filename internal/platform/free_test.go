//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreeSpace(t *testing.T) {
	dir := t.TempDir()
	var h Host
	free, err := h.FreeSpace(dir)
	if err != nil || free == 0 {
		t.Fatalf("FreeSpace(%s) = %d, %v", dir, free, err)
	}
	// A folder that isn't made yet is on its nearest parent's volume.
	if got, err := h.FreeSpace(filepath.Join(dir, "a", "b")); err != nil || got == 0 {
		t.Errorf("a missing folder: FreeSpace = %d, %v", got, err)
	}
	// A file in the way is an error, not its parent's volume.
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.FreeSpace(filepath.Join(f, "x")); err == nil {
		t.Error("a path under a file: no error")
	}
}

func TestMemory(t *testing.T) {
	m, err := Host{}.Memory()
	if err != nil || m < 1<<28 {
		t.Errorf("Memory() = %d, %v", m, err)
	}
}

// TestHostOverrides: a test build's Host reads the overrides set, a
// release's reads the host's own whatever is set.
func TestHostOverrides(t *testing.T) {
	env := map[string]string{TestCPUs: "2", TestMemoryGiB: "7", TestFreeGiB: "10.5"}
	getenv := func(k string) string { return env[k] }
	test := NewHost(true, getenv)
	if got := test.CPUs(); got != 2 {
		t.Errorf("test CPUs = %d, want 2", got)
	}
	if got, err := test.Memory(); err != nil || got != 7<<30 {
		t.Errorf("test Memory = %d, %v, want %d", got, err, uint64(7<<30))
	}
	if got, err := test.FreeSpace("/nowhere/at/all"); err != nil || got != 21<<29 {
		t.Errorf("test FreeSpace = %d, %v, want %d", got, err, uint64(21<<29))
	}

	release, host := NewHost(false, getenv), Host{}
	if release.CPUs() != host.CPUs() {
		t.Errorf("release CPUs = %d, want the host's %d", release.CPUs(), host.CPUs())
	}
	hm, _ := host.Memory()
	if m, err := release.Memory(); err != nil || m != hm {
		t.Errorf("release Memory = %d, %v, want the host's %d", m, err, hm)
	}
	if _, err := release.FreeSpace("/nowhere/at/all"); err != nil {
		t.Errorf("release FreeSpace of a missing path: %v, want its parent's", err)
	}

	// Unset falls through to the host; a bad number is an error.
	env = map[string]string{TestMemoryGiB: "lots", TestCPUs: "none"}
	if test.CPUs() != host.CPUs() {
		t.Errorf("bad CPUs: %d, want the host's", test.CPUs())
	}
	if _, err := test.Memory(); err == nil {
		t.Error("bad memory: no error")
	}
	if _, err := test.FreeSpace(t.TempDir()); err != nil {
		t.Errorf("unset free space: %v", err)
	}
}
