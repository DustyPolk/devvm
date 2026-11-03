//go:build darwin || linux

package platform

import (
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
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
	// sh ignores its extra arguments (GNU sleep would exit on them), and
	// stays itself while sleep isn't its last command.
	cmd := exec.CommandContext(t.Context(), "sh", "-c", "sleep 30; :", "sh", "a b", "")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	// Right after Start, the child may not have exec'd sleep yet: Linux
	// shows the forked Go process's arguments until it does.
	want := []string{"sh", "-c", "sleep 30; :", "sh", "a b", ""}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got, err = ProcessArgs(cmd.Process.Pid); err == nil && slices.Equal(got, want) {
			return
		}
	}
	t.Errorf("ProcessArgs = %q, %v", got, err)
}

// A process that has ended but isn't reaped yet (a zombie) has no
// arguments to read, as one that's ending has none: an error, never an
// empty list taken for another process's.
func TestProcessArgsEnded(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "true")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Wait() }()
	pid := cmd.Process.Pid
	// It's a zombie once its arguments are no longer true's.
	var got []string
	var err error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if got, err = ProcessArgs(pid); err != nil || !slices.Equal(got, []string{"true"}) && !slices.Equal(got, os.Args) {
			break
		}
	}
	if err == nil {
		t.Errorf("ProcessArgs of an ended process = %q", got)
	}
}
