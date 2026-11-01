package state

import (
	"errors"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

func TestParseDuration(t *testing.T) {
	day := 24 * time.Hour
	for in, want := range map[string]time.Duration{"30d": 30 * day, "2w": 14 * day, "12h": 12 * time.Hour, "1d": day, "30m": 30 * time.Minute} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "0d", "-1d", "30", "30s", "1h30m", "1.5d", "30 d", "+3d", "999999d", "30D", "forever", "off"} {
		_, err := ParseDuration(in)
		if m, ok := errors.AsType[msg.M](err); !ok || m.Names() != "bad_duration value="+in {
			t.Errorf("ParseDuration(%q) = %v; want bad_duration", in, err)
		}
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"64K": 64 << 10, "500M": 500 << 20, "20G": 20 << 30, "1T": 1 << 40, "1024T": 1 << 50} {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "G", "0G", "-1G", "20", "20g", "20GB", "20GiB", "1.5G", "20 G", "+3G", "1025T", "99999999999999999999G"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q): want an error", in)
		}
	}
}
