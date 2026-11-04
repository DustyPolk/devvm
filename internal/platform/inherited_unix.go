//go:build unix

package platform

import (
	"os"
	"strconv"
	"syscall"
)

// CloseInheritedOnExec marks every file devvm was given past its standard
// input, output and error close-on-exec, so each process it starts gets
// only those three and the files it's handed. A terminal, an editor or a
// test harness can give devvm others without close-on-exec (a pty, a
// pipe's end), and a background process holding one keeps that terminal
// or pipe open for as long as it runs. devvm itself keeps them open. Go
// opens its own files close-on-exec, so this runs once, as devvm starts.
func CloseInheritedOnExec() {
	// /dev/fd lists this process's open files, on macOS and on Linux.
	entries, err := os.ReadDir("/dev/fd")
	if err != nil {
		return
	}
	for _, e := range entries {
		if fd, err := strconv.Atoi(e.Name()); err == nil && fd > 2 {
			syscall.CloseOnExec(fd)
		}
	}
}
