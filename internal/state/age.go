package state

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// errAge is an age that isn't a whole number of days, weeks or hours.
var errAge = errors.New("an age is a whole number of days, weeks or hours, such as 30d, 2w or 12h")

// ParseAge reads an age as devvm's settings and flags take one: a whole
// number of days (30d), weeks (2w) or hours (12h), more than zero.
func ParseAge(s string) (time.Duration, error) {
	units := map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour, "h": time.Hour}
	if len(s) < 2 {
		return 0, errAge
	}
	unit, ok := units[s[len(s)-1:]]
	digits := s[:len(s)-1]
	if !ok || strings.TrimLeft(digits, "0123456789") != "" {
		return 0, errAge
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 || n > 100000 {
		return 0, errAge
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
