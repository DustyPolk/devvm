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
	p, err := StartDetachedProcess(path, args, out)
	if err != nil {
		return 0, err
	}
	pid := p.Pid
	return pid, p.Release()
}

// StartDetachedProcess is StartDetached, returning the process itself:
// signals sent through it reach that process alone, never another given
// its pid later, as devvm hasn't waited for it.
func StartDetachedProcess(path string, args []string, out *os.File) (*os.Process, error) {
	cmd := exec.Command(path, args...) //nolint:gosec,noctx // devvm's own binary; it outlives the run's context
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
