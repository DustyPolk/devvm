package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// replaceProcess runs argv in this process's place, as a shell's exec
// does, so a signal for the stub reaches the command and its output ends
// with it. It returns only when that fails.
func replaceProcess(argv, extra []string) int {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		_, _ = os.Stderr.WriteString("stub: " + err.Error() + "\n") //nolint:forbidigo // a stub's output is the command's
		return 127
	}
	err = syscall.Exec(path, argv, append(os.Environ(), extra...)) //nolint:gosec // the command the stub stands in for
	_, _ = os.Stderr.WriteString("stub: " + err.Error() + "\n")    //nolint:forbidigo // a stub's output is the command's
	return 126
}

// output runs argv, the guest's own command, its input the call's, and
// returns its output and exit status.
func (c *call) output(argv ...string) ([]byte, int) {
	var out bytes.Buffer
	cmd := exec.CommandContext(c.ctx, argv[0], argv[1:]...) //nolint:gosec // the guest's command, which the stub runs on the host
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.stdin, &out, c.stderr
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return out.Bytes(), exit.ExitCode()
	} else if err != nil {
		c.say("stub: " + err.Error())
		return out.Bytes(), 127
	}
	return out.Bytes(), 0
}

// interrupt sends the process that ran the stub SIGINT, as Ctrl+C does.
func (c *call) interrupt() {
	_ = syscall.Kill(c.ppid, syscall.SIGINT) //nolint:forbidigo // the stub's parent, devvm, as the terminal would
}
