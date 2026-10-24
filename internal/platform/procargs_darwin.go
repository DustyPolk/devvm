package platform

import (
	"bytes"
	"encoding/binary"
	"errors"

	"golang.org/x/sys/unix"
)

// ProcessArgs returns a running process's arguments, its program's name
// first, from the kernel (kern.procargs2).
func ProcessArgs(pid int) ([]string, error) {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	// argc, the executable's path, NUL padding, then argc NUL-ended
	// arguments (the environment follows).
	if len(b) < 4 {
		return nil, errors.New("short kern.procargs2")
	}
	argc := int(binary.LittleEndian.Uint32(b[:4]))
	rest := b[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil, errors.New("bad kern.procargs2")
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	args := make([]string, 0, argc)
	for range argc {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return nil, errors.New("bad kern.procargs2")
		}
		args = append(args, string(rest[:j]))
		rest = rest[j+1:]
	}
	return args, nil
}
