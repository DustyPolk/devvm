package state

import (
	"testing"
	"time"
)

func TestParseAge(t *testing.T) {
	day := 24 * time.Hour
	for in, want := range map[string]time.Duration{"30d": 30 * day, "2w": 14 * day, "12h": 12 * time.Hour, "1d": day} {
		if got, err := ParseAge(in); err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "0d", "-1d", "30", "30m", "1.5d", "30 d", "+3d", "999999d", "30D"} {
		if _, err := ParseAge(in); err == nil {
			t.Errorf("ParseAge(%q): want an error", in)
		}
	}
}
