//go:build unix

package platform

import "syscall"

// ShareFileLimit gives the processes devvm starts from now on its own
// limit on open files. Go raises its soft limit to the hard one as it
// starts, but gives each process it starts the soft limit it started with
// (256 on macOS by default), unless the program sets the limit itself:
// setting it to what it is now is enough.
func ShareFileLimit() error {
	var r syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &r); err != nil {
		return err
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &r)
}
