package platform

import "golang.org/x/sys/unix"

// MacOSVersion returns macOS's version (26.0), or "" when it can't be read.
func MacOSVersion() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
