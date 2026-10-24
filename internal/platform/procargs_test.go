//go:build darwin || linux

package platform

import (
	"os"
	"os/exec"
	"slices"
	"testing"
)

// A process's arguments, as it was started with them.
func TestProcessArgs(t *testing.T) {
	got, err := ProcessArgs(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got[1:], os.Args[1:]) {
		t.Errorf("ProcessArgs = %q, want %q", got, os.Args)
	}
	cmd := exec.CommandContext(t.Context(), "sleep", "30", "a b", "")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if got, err := ProcessArgs(cmd.Process.Pid); err != nil || !slices.Equal(got, []string{"sleep", "30", "a b", ""}) {
		t.Errorf("ProcessArgs = %q, %v", got, err)
	}
}
