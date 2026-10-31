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
