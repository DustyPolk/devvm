package platform

import "golang.org/x/sys/unix"

// ephemeralFirst returns the first port of macOS's ephemeral range
// (net.inet.ip.portrange.first), or 49152, its default.
func ephemeralFirst() (int, bool) {
	n, err := unix.SysctlUint32("net.inet.ip.portrange.first")
	if err != nil || n == 0 || n > 65535 {
		return 49152, true
	}
	return int(n), true
}
