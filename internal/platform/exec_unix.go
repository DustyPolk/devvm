//go:build unix

package platform

import "syscall"

// Exec replaces devvm with the program at path, run with argv (argv[0]
// included) and env. It returns only when that fails.
func Exec(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env) //nolint:gosec // the caller chose the program
}
