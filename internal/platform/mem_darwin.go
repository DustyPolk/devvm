package platform

import "golang.org/x/sys/unix"

// memory returns the host's memory in bytes.
func memory() (uint64, error) { return unix.SysctlUint64("hw.memsize") }
