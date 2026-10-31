//go:build darwin || linux

package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFreeSpace(t *testing.T) {
	dir := t.TempDir()
	free, err := FreeSpace(dir)
	if err != nil || free == 0 {
		t.Fatalf("FreeSpace(%s) = %d, %v", dir, free, err)
	}
	// A folder that isn't made yet is on its nearest parent's volume.
	if got, err := FreeSpace(filepath.Join(dir, "a", "b")); err != nil || got == 0 {
		t.Errorf("a missing folder: FreeSpace = %d, %v", got, err)
	}
	// A file in the way is an error, not its parent's volume.
	f := filepath.Join(dir, "f")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FreeSpace(filepath.Join(f, "x")); err == nil {
		t.Error("a path under a file: no error")
	}
}
