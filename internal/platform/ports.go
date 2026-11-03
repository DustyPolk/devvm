package platform

import (
	"context"
	"errors"
	"math/rand/v2"
	"strconv"
	"strings"
)

// PortRange is the ports from Lo to Hi, both included.
type PortRange struct{ Lo, Hi int }

// The ports devvm chooses from: from portsLo to below the host's ephemeral
// range, which the system gives outgoing connections (a listen on a port
// a live connection holds fails on Linux), or to portsFallbackHi when
// that range can't be read or leaves fewer than portsMin.
const (
	portsLo         = 20000
	portsFallbackHi = 32767
	portsMin        = 1000
)

// PortsBelow returns the range devvm chooses ports from on a host whose
// ephemeral range starts at first (ok false: unknown).
func PortsBelow(first int, ok bool) PortRange {
	if !ok || first-1-portsLo+1 < portsMin {
		return PortRange{portsLo, portsFallbackHi}
	}
	return PortRange{portsLo, first - 1}
}

// ChoosablePorts returns the range devvm chooses ports from on this host
// (PortsBelow its ephemeral range).
func ChoosablePorts() PortRange { return PortsBelow(ephemeralFirst()) }

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

// FreePort returns a random port of r that usable accepts, trying 1000.
func FreePort(ctx context.Context, r PortRange, usable func(ctx context.Context, port int) bool) (int, error) {
	for range 1000 {
		port := r.Lo + rand.IntN(r.Hi-r.Lo+1) //nolint:gosec // a port, not a secret
		if usable(ctx, port) {
			return port, nil
		}
	}
	return 0, errors.New("no free port on 127.0.0.1")
}
