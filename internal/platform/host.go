package platform

import (
	"fmt"
	"runtime"
	"strconv"
)

// The environment variables a test build's Host reads in place of the
// host's own: its CPUs, its memory in GiB, and the free space in GiB on
// every volume.
const (
	TestCPUs      = "DEVVM_TEST_CPUS"
	TestMemoryGiB = "DEVVM_TEST_MEMORY_GIB"
	TestFreeGiB   = "DEVVM_TEST_FREE_GIB"
)

// Host reads this machine's CPUs, memory and free space. The zero Host
// reads the machine's own; a test build's (NewHost) reads the overrides
// set in its environment instead, so the CLI tests can stand in for a
// host of any size.
type Host struct {
	getenv func(string) string // nil: no overrides
}

// NewHost returns the Host for a build: with test (a test build, as
// tests/e2e.sh and the CLI tests make), the overrides getenv has set
// count; without it (a release) none do.
func NewHost(test bool, getenv func(string) string) Host {
	if !test {
		return Host{}
	}
	return Host{getenv: getenv}
}

// override returns the value of a test build's override name, "" for
// none.
func (h Host) override(name string) string {
	if h.getenv == nil {
		return ""
	}
	return h.getenv(name)
}

// CPUs returns the host's CPUs; a test build's DEVVM_TEST_CPUS, when it's
// a positive number.
func (h Host) CPUs() int {
	if n, err := strconv.Atoi(h.override(TestCPUs)); err == nil && n > 0 {
		return n
	}
	return runtime.NumCPU()
}

// Memory returns the host's memory in bytes; a test build's
// DEVVM_TEST_MEMORY_GIB, in GiB.
func (h Host) Memory() (uint64, error) {
	if v := h.override(TestMemoryGiB); v != "" {
		return gib(TestMemoryGiB, v)
	}
	return memory()
}

// FreeSpace returns the free space, in bytes, that an unprivileged user
// can write on the volume holding path, or on its nearest folder that
// exists when path doesn't yet; a test build's DEVVM_TEST_FREE_GIB, in
// GiB, for every path.
func (h Host) FreeSpace(path string) (uint64, error) {
	if v := h.override(TestFreeGiB); v != "" {
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
