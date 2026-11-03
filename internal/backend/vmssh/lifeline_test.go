package vmssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A forwarder needs the VM's lifeline, naming a process that runs as it
// starts (daemon.Lifeline): without one, or with one whose process is
// gone, it doesn't run.
func TestLifelineMain(t *testing.T) {
	reserved := func(string) (map[int]bool, error) { return nil, nil }
	base := []string{"-vm", "v", "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-port", "1"}
	if err := LifelineMain(t.Context(), "_x-ports", base, nil, reserved, &strings.Builder{}); err == nil {
		t.Error("no lifeline: ran")
	}
	gone := filepath.Join(t.TempDir(), "ha.pid")
	if err := LifelineMain(t.Context(), "_x-ports", append(base, "-lifeline", gone), nil, reserved, &strings.Builder{}); err == nil {
		t.Error("an empty lifeline: ran")
	}
	if err := os.WriteFile(gone, []byte("999999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LifelineMain(t.Context(), "_x-ports", append(base, "-lifeline", gone), nil, reserved, &strings.Builder{}); err == nil {
		t.Error("a lifeline whose process is gone: ran")
	}
}
