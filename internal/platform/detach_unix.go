//go:build unix

package platform

import (
	"os"
	"os/exec"
	"syscall"
)

// StartDetached starts path with args in a session of its own, so it
// outlives devvm and the terminal it ran in, with no input and its output
// in out. It returns the process's pid without waiting for it.
func StartDetached(path string, args []string, out *os.File) (int, error) {
	cmd := exec.Command(path, args...) //nolint:gosec,noctx // devvm's own binary; it outlives the run's context
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}
