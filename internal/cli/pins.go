package cli

import (
	"runtime"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/backend/lima"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/platform"
)

// pinsCheck is doctor's check of .git protection on a Linux host, which
// locks .git in a user namespace around each VM's QEMU: whether the host
// lets devvm do that, and why not. Tests set its fields.
type pinsCheck struct {
	// Probe is the backend's probe (lima.Lima's Pins): nil when this host
	// pins, else why not; a nil Probe is a host that doesn't lock this way
	// (macOS).
	Probe func() error
	// Limit is why the kernel's settings refuse it (platform.UsernsLimit).
	Limit func() string
}

// newPinsCheck is the check of the host's pins, Lima's probe's: on a
// Linux host only.
func newPinsCheck(reg *backend.Registry) pinsCheck {
	c := pinsCheck{Limit: platform.UsernsLimit}
	if runtime.GOOS != "linux" || reg == nil {
		return c
	}
	if b, ok := reg.Lookup(lima.Name); ok {
		if l, ok := backend.As[*lima.Lima](b); ok {
			c.Probe = l.Pins
		}
	}
	return c
}

// warning is doctor's warning when this host can't lock .git: what the
// kernel's settings say, or the probe's error.
func (c pinsCheck) warning() (msg.M, bool) {
	if c.Probe == nil {
		return msg.M{}, false
	}
	err := c.Probe()
	if err == nil {
		return msg.M{}, false
	}
	switch c.Limit() {
	case "apparmor":
		return msg.DoctorPinsApparmor(), true
	case "sysctl":
		return msg.DoctorPinsSysctl(), true
	case "max":
		return msg.DoctorPinsMax(), true
	}
	return msg.DoctorPinsFailed(err.Error()).Because(err), true
}
