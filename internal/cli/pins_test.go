package cli

import (
	"errors"
	"testing"
)

// doctor's warning names why this host can't lock .git: the kernel's
// setting, else the probe's error; none where it can, or off Linux.
func TestPinsWarning(t *testing.T) {
	refused := errors.New("operation not permitted")
	for _, tc := range []struct {
		probe func() error
		limit string
		want  string
	}{
		{nil, "", ""},
		{func() error { return nil }, "apparmor", ""},
		{func() error { return refused }, "apparmor", "doctor_pins_apparmor"},
		{func() error { return refused }, "sysctl", "doctor_pins_sysctl"},
		{func() error { return refused }, "max", "doctor_pins_max"},
		{func() error { return refused }, "", "doctor_pins_failed reason=operation not permitted"},
	} {
		c := pinsCheck{Probe: tc.probe, Limit: func() string { return tc.limit }}
		m, ok := c.warning()
		if got := m.Names(); ok != (tc.want != "") || ok && got != tc.want {
			t.Errorf("limit %q: %q (%t), want %q", tc.limit, got, ok, tc.want)
		}
	}
	if c := newPinsCheck(nil); c.Probe != nil || c.Limit == nil {
		t.Error("a check without a registry probes")
	}
}
