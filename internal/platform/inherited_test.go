//go:build unix

package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// inheritedChild, set in the environment, makes TestCloseInheritedOnExec
// the child it starts: given a file on descriptor 3 without close-on-exec,
// as a terminal or testscript's pty hands devvm one, it starts a detached
// process and logs whether that process holds descriptor 3.
const inheritedChild = "DEVVM_PLATFORM_TEST_INHERITED_CHILD"

// A file devvm was given past its standard three stays open in devvm but
// reaches none of the processes it starts: a detached one holding it would
// keep the terminal or pipe behind it open for as long as it runs.
func TestCloseInheritedOnExec(t *testing.T) {
	if os.Getenv(inheritedChild) != "" {
		inheritedChildRun(t)
		return
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	extra, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = extra.Close() }()
	cmd := exec.CommandContext(t.Context(), self, "-test.v", "-test.run=^TestCloseInheritedOnExec$") //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), inheritedChild+"=1")
	cmd.ExtraFiles = []*os.File{extra}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !strings.Contains(string(out), "kept=true held=false") {
		t.Errorf("descriptor 3 reached the detached process, or devvm lost it: %s", out)
	}
}

// inheritedChildRun is the child's side: it marks its inherited files,
// starts a detached sleep and checks descriptor 3 in it and in itself.
func inheritedChildRun(t *testing.T) {
	t.Helper()
	CloseInheritedOnExec()
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, 3, syscall.F_GETFD, 0)
	kept := errno == 0
	cmd := exec.CommandContext(t.Context(), "sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	t.Logf("kept=%t held=%t", kept, holds(t, cmd.Process.Pid, 3))
}

// holds reports whether the process with that pid has descriptor fd open:
// its /proc entry on Linux, lsof elsewhere.
func holds(t *testing.T, pid, fd int) bool {
	t.Helper()
	if runtime.GOOS == "linux" {
		_, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/fd/" + strconv.Itoa(fd))
		return err == nil
	}
	out, _ := exec.CommandContext(t.Context(), "lsof", "-a", "-p", strconv.Itoa(pid), "-d", strconv.Itoa(fd), "-F", "f").Output() //nolint:gosec // the test's own child
	return strings.Contains(string(out), "\nf"+strconv.Itoa(fd))
}
