package platform

import (
	"context"
	"testing"
)

// The range devvm chooses from ends below the host's ephemeral range, or
// falls back below 32768 when that's unknown or leaves too few.
func TestPortsBelow(t *testing.T) {
	for _, tc := range []struct {
		first int
		ok    bool
		want  PortRange
	}{
		{32768, true, PortRange{20000, 32767}}, // Linux's default
		{49152, true, PortRange{20000, 49151}}, // macOS's
		{21000, true, PortRange{20000, 20999}},
		{20999, true, PortRange{20000, 32767}}, // too few below it
		{1024, true, PortRange{20000, 32767}},
		{0, false, PortRange{20000, 32767}}, // unreadable
	} {
		got := PortsBelow(tc.first, tc.ok)
		if got != tc.want {
			t.Errorf("PortsBelow(%d, %v) = %v, want %v", tc.first, tc.ok, got, tc.want)
		}
		if tc.ok && got.Hi >= tc.first && got != (PortRange{20000, 32767}) {
			t.Errorf("PortsBelow(%d) overlaps the ephemeral range: %v", tc.first, got)
		}
	}
	if r := ChoosablePorts(); r.Lo != 20000 || r.Hi < r.Lo+portsMin-1 {
		t.Errorf("ChoosablePorts = %v", r)
	}
}

func TestParsePortRangeFirst(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"32768\t60999\n", 32768, true},
		{"40000 65000", 40000, true},
		{"", 0, false},
		{"x 1", 0, false},
		{"70000 80000", 70000, false},
	} {
		if n, ok := parsePortRangeFirst(tc.in); ok != tc.ok || ok && n != tc.want {
			t.Errorf("parsePortRangeFirst(%q) = %d, %v", tc.in, n, ok)
		}
	}
}

// FreePort returns a port of the range usable accepts, and fails when
// none is.
func TestFreePort(t *testing.T) {
	r := PortRange{30000, 30009}
	for range 100 {
		p, err := FreePort(t.Context(), r, func(_ context.Context, p int) bool { return p%2 == 1 })
		if err != nil || p < r.Lo || p > r.Hi || p%2 == 0 {
			t.Fatalf("FreePort = %d, %v", p, err)
		}
	}
	if _, err := FreePort(t.Context(), r, func(context.Context, int) bool { return false }); err == nil {
		t.Error("FreePort with nothing usable: no error")
	}
}
