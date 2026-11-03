//go:build !linux && !darwin

package platform

// ephemeralFirst can't read the ephemeral range here: PortsBelow falls
// back to a range below 32768.
func ephemeralFirst() (int, bool) { return 0, false }
