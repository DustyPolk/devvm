package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePasteTool puts a script named tool first on PATH that says text on
// stderr and exits 1, as the clipboard tools do when they fail.
func fakePasteTool(t *testing.T, tool, text string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\necho " + text + " >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o700); err != nil { //nolint:gosec // the test's own script, to run
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// A listing wl-paste fails with its word for an empty clipboard is an
// empty clipboard; one it fails otherwise, as without a compositor at
// WAYLAND_DISPLAY, is the tool's failure, with what it said. xclip's
// words aren't known, so its every failed listing is an empty clipboard.
func TestReadClipToolFailed(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-9")
	fakePasteTool(t, "wl-paste", "'Nothing is copied'")
	if _, err := ReadClip(t.Context(), 1<<20); !errors.Is(err, ErrClipEmpty) {
		t.Errorf("wl-paste on an empty clipboard: %v", err)
	}
	fakePasteTool(t, "wl-paste", "'Failed to connect to a Wayland server'")
	var failed *ClipToolFailed
	if _, err := ReadClip(t.Context(), 1<<20); !errors.As(err, &failed) || failed.Tool != "wl-paste" || !strings.Contains(failed.Stderr, "Failed to connect") {
		t.Errorf("wl-paste without a compositor: %v", err)
	}
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":9")
	fakePasteTool(t, "xclip", "\"Error: Can't open display: :9\"")
	if _, err := ReadClip(t.Context(), 1<<20); !errors.Is(err, ErrClipEmpty) {
		t.Errorf("xclip failing: %v", err)
	}
}
