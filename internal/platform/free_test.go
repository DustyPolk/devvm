//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// TestListTimeout: a test build's positive duration counts; a release's,
// a bad one, a non-positive one or none give the default.
func TestListTimeout(t *testing.T) {
	const def = 30 * time.Second
	for _, c := range []struct {
		test bool
		v    string
		want time.Duration
	}{
		{true, "1s", time.Second},
		{true, "10m,lima:1s", time.Second},
		{true, "lima:1s,10m", time.Second},
		{true, "10m,smolvm:1s", 10 * time.Minute},
		{true, "smolvm:1s", def},
		{true, "", def},
		{true, "soon", def},
		{true, "0s", def},
		{true, "-1s", def},
		{false, "1s", def},
	} {
		h := NewHost(c.test, func(k string) string { return map[string]string{TestListWait: c.v}[k] })
		if got := h.ListTimeout("lima", def); got != c.want {
			t.Errorf("test=%v %q: %v, want %v", c.test, c.v, got, c.want)
		}
	}
}
