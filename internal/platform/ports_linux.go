package platform

import (
	"os"
	"strconv"
	"strings"
)

// ephemeralFirst returns the first port of Linux's ephemeral range
// (net.ipv4.ip_local_port_range).
func ephemeralFirst() (int, bool) {
	b, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		return 0, false
	}
	return parsePortRangeFirst(string(b))
}

// parsePortRangeFirst reads the first field of Linux's
// ip_local_port_range ("32768\t60999").
func parsePortRangeFirst(s string) (int, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(f[0])
	return n, err == nil && n > 0 && n < 65536
}
