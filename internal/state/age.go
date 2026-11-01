package state

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

// ParseDuration reads a duration as every devvm flag and setting takes
// one, an age or a lifetime: a whole number, more than zero, and a unit,
// minutes (30m), hours (12h), days (30d) or weeks (2w). A bad one is
// bad_duration, naming it.
func ParseDuration(s string) (time.Duration, error) {
	const day = 24 * time.Hour
	units := map[string]time.Duration{"m": time.Minute, "h": time.Hour, "d": day, "w": 7 * day}
	bad := msg.BadDuration(msg.Shown(s))
	if len(s) < 2 {
		return 0, bad
	}
	unit, ok := units[s[len(s)-1:]]
	digits := s[:len(s)-1]
	if !ok || strings.TrimLeft(digits, "0123456789") != "" {
		return 0, bad
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 || n > 100000 {
		return 0, bad
	}
	return time.Duration(n) * unit, nil
}

// errSize is a size that isn't a whole number of K, M, G or T.
var errSize = errors.New("a size is a whole number of K, M, G or T (1024 each), such as 500M or 20G")

// ParseSize reads a size as devvm's settings take one: a whole number of
// KiB (64K), MiB (500M), GiB (20G) or TiB (1T), more than zero, as devvm
// space shows sizes.
func ParseSize(s string) (int64, error) {
	units := map[string]int64{"K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40}
	if len(s) < 2 {
		return 0, errSize
	}
	unit, ok := units[s[len(s)-1:]]
	digits := s[:len(s)-1]
	if !ok || strings.TrimLeft(digits, "0123456789") != "" {
		return 0, errSize
	}
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || n <= 0 || n > 1<<50/unit {
		return 0, errSize
	}
	return n * unit, nil
}
