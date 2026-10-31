package platform

import "golang.org/x/sys/unix"

// memory returns the host's memory in bytes.
func memory() (uint64, error) {
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err != nil {
		return 0, err
	}
	return uint64(si.Totalram) * uint64(si.Unit), nil //nolint:unconvert // their types differ between architectures
}
