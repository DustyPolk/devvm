package platform

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// ProcessArgs returns a running process's arguments, its program's name
// first, from /proc. One that's ending, or ended and not yet reaped, has
// none there: an error, as on macOS.
func ProcessArgs(pid int) ([]string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("no arguments: the process is ending, or a kernel thread")
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), nil
}
