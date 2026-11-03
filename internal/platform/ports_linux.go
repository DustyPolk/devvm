package platform

import "os"

// ephemeralFirst returns the first port of Linux's ephemeral range
// (net.ipv4.ip_local_port_range).
func ephemeralFirst() (int, bool) {
	b, err := os.ReadFile("/proc/sys/net/ipv4/ip_local_port_range")
	if err != nil {
		return 0, false
	}
	return parsePortRangeFirst(string(b))
}
