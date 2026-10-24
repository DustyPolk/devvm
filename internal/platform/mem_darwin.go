package platform

import "golang.org/x/sys/unix"

// Memory returns the host's memory in bytes.
func Memory() (uint64, error) { return unix.SysctlUint64("hw.memsize") }
