package platform

import "testing"

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
