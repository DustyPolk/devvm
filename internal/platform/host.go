package platform

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The environment variables whose values a test build's Host takes in
// place of the host's own (Overrides): its CPUs, its memory in GiB, and
// the free space in GiB on every volume; and, in place of a timeout's own
// length, how long devvm waits for a backend's listings (see
// ListTimeout).
const (
	TestCPUs      = "DEVVM_TEST_CPUS"
	TestMemoryGiB = "DEVVM_TEST_MEMORY_GIB"
	TestFreeGiB   = "DEVVM_TEST_FREE_GIB"
	TestListWait  = "DEVVM_TEST_LIST_TIMEOUT"
)

// Overrides are a test build's stand-ins for what Host reads of this
// machine, each the value of its environment variable above, as
// internal/cli reads them; "" leaves the machine's own. A release has
// none.
type Overrides struct {
	CPUs, MemoryGiB, FreeGiB, ListTimeout string
}

// ListTimeout returns def, how long devvm waits for backend's listings;
// in a test build, DEVVM_TEST_LIST_TIMEOUT's, so the CLI tests don't
// depend on how loaded the machine is. Its value is comma-separated
// positive durations, each for every backend ("10m") or for one
// ("smolvm:1s"); one backend's wins.
func (h Host) ListTimeout(backend string, def time.Duration) time.Duration {
	all, one := time.Duration(0), time.Duration(0)
	for _, e := range strings.Split(h.o.ListTimeout, ",") {
		name, v, named := strings.Cut(e, ":")
		if !named {
			name, v = "", e
		}
		d, err := time.ParseDuration(v)
		switch {
		case err != nil || d <= 0:
		case named && name == backend:
			one = d
		case !named:
			all = d
		}
	}
	switch {
	case one > 0:
		return one
	case all > 0:
		return all
	}
	return def
}

// Host reads this machine's CPUs, memory and free space. The zero Host
// reads the machine's own; a test build's (NewHost) takes its overrides
// instead, so the CLI tests can stand in for a host of any size.
type Host struct {
	o Overrides
}

// NewHost returns the Host that takes o in place of what it reads of
// this machine: a test build's overrides, none in a release.
func NewHost(o Overrides) Host { return Host{o: o} }

// CPUs returns the host's CPUs; a test build's DEVVM_TEST_CPUS, when it's
// a positive number.
func (h Host) CPUs() int {
	if n, err := strconv.Atoi(h.o.CPUs); err == nil && n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// Memory returns the host's memory in bytes; a test build's
// DEVVM_TEST_MEMORY_GIB, in GiB.
func (h Host) Memory() (uint64, error) {
	if v := h.o.MemoryGiB; v != "" {
		return gib(TestMemoryGiB, v)
	}
	return memory()
}

// FreeSpace returns the free space, in bytes, that an unprivileged user
// can write on the volume holding path, or on its nearest folder that
// exists when path doesn't yet; a test build's DEVVM_TEST_FREE_GIB, in
// GiB, for every path.
func (h Host) FreeSpace(path string) (uint64, error) {
	if v := h.o.FreeGiB; v != "" {
		return gib(TestFreeGiB, v)
	}
	return freeSpace(path)
}

// gib returns v, a number of GiB that override name set, in bytes.
func gib(name, v string) (uint64, error) {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("%s=%q: not a number of GiB", name, v)
	}
	return uint64(f * (1 << 30)), nil
}
