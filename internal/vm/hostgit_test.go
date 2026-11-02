package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// devvm's git on the host never runs a repo's fsmonitor, which a repo's
// config can point at any program.
func TestHostGitNoFsmonitor(t *testing.T) {
	gitEnv(t)
	dir := t.TempDir()
	gitIn(t, dir, "init", "-q")
	ran := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+ran+"'\n"), 0o700); err != nil { //nolint:gosec // the test's own script
		t.Fatal(err)
	}
	gitIn(t, dir, "config", "core.fsmonitor", hook)
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "f")
	// Plain git runs it.
	if err := os.Remove(ran); err != nil {
		t.Fatalf("git didn't run the fsmonitor: %v", err)
	}
	if _, err := hostGit(t.Context(), dir, "status", "--porcelain", "--untracked-files=no"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("ran the repo's fsmonitor")
	}
}

// A --clone VM's host git runs none of the repo's hooks: not pre-push or
// reference-transaction as the clone is seeded, nor reference-transaction
// as rm fetches the clone's commits, saves them and removes the remote.
func TestCloneRunsNoHooks(t *testing.T) {
	e, _, vmfs := cloneEnv(t)
	ran := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "hook")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+ran+"'\ncat >/dev/null\n"), 0o700); err != nil { //nolint:gosec // the test's own script
		t.Fatal(err)
	}
	for _, h := range []string{"pre-push", "reference-transaction"} {
		if err := os.Symlink(hook, filepath.Join(e.Dir, ".git", "hooks", h)); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, e.Dir, "update-ref", "refs/heads/plain", "HEAD")
	if err := os.Remove(ran); err != nil {
		t.Fatalf("plain git didn't run reference-transaction: %v", err)
	}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Clone: true})
	if _, err := os.Stat(ran); err == nil {
		t.Error("seeding the clone ran a hook")
	}
	gitIn(t, vmfs+e.Dir, "commit", "-q", "--allow-empty", "-m", "from-vm")
	answers(t, e, true)
	if err := (Lifecycle{e}).Rm(t.Context(), RmOptions{Force: true}); err != nil {
		t.Fatal(names(err))
	}
	if got := gitIn(t, e.Dir, "log", "--format=%s", "-1", "refs/heads/saved/devvm/main"); got != "from-vm" {
		t.Errorf("fetched %q", got)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Errorf("rm of %s ran a hook", st.Name())
	}
}
