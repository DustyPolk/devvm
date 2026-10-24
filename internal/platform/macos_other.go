//go:build !darwin

package platform

// MacOSVersion returns macOS's version: "" on other systems.
func MacOSVersion() string { return "" }
