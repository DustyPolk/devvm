package platform

import (
	"os"
	"strconv"
	"strings"
)

// ProcessArgs returns a running process's arguments, its program's name
// first, from /proc.
func ProcessArgs(pid int) ([]string, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"), nil
}
