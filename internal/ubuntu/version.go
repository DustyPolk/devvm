package ubuntu

import (
	"cmp"
	"errors"
	"strconv"
	"strings"
)

// ErrVersion is a package version that isn't one by Debian policy (5.6.12):
// [epoch:]upstream[-revision], the epoch a number, upstream not empty.
var ErrVersion = errors.New("not a Debian package version")

// CompareVersions orders Debian package versions a and b as dpkg
// --compare-versions does: below zero when a is older, zero when they're
// the same version, above zero when a is newer. The epoch counts first,
// then upstream, then the revision, each part's letters before its other
// characters and ~ before everything, even the part's end, so 1.0~rc1 is
// older than 1.0. It fails with ErrVersion for a version that isn't one.
func CompareVersions(a, b string) (int, error) {
	va, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	vb, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	if c := cmp.Compare(va.epoch, vb.epoch); c != 0 {
		return c, nil
	}
	if c := compareRevisions(va.upstream, vb.upstream); c != 0 {
		return c, nil
	}
	return compareRevisions(va.revision, vb.revision), nil
}

type version struct {
	epoch              int64
	upstream, revision string
}

// parseVersion splits s at its first colon, the epoch's, and its last
// hyphen, the revision's, checking each part's characters as dpkg does.
func parseVersion(s string) (version, error) {
	var v version
	rest := s
	if e, after, ok := strings.Cut(s, ":"); ok {
		n, err := strconv.ParseInt(e, 10, 32)
		if err != nil || n < 0 || strings.HasPrefix(e, "+") {
			return version{}, ErrVersion
		}
		v.epoch, rest = n, after
	}
	v.upstream = rest
	if i := strings.LastIndexByte(rest, '-'); i >= 0 {
		v.upstream, v.revision = rest[:i], rest[i+1:]
		if v.revision == "" {
			return version{}, ErrVersion
		}
	}
	if v.upstream == "" || !validPart(v.upstream, ".+~-:") || !validPart(v.revision, ".+~") {
		return version{}, ErrVersion
	}
	return v, nil
}

// validPart reports whether s holds only ASCII letters and digits and the
// punctuation of extra.
func validPart(s, extra string) bool {
	for _, c := range []byte(s) {
		if !isDigit(c) && !isLetter(c) && strings.IndexByte(extra, c) < 0 {
			return false
		}
	}
	return true
}

// compareRevisions is dpkg's verrevcmp: a and b compared a run of
// non-digits, then a run of digits, at a time; the non-digits by order,
// the digits as numbers.
func compareRevisions(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			if c := cmp.Compare(order(a, i), order(b, j)); c != 0 {
				return c
			}
			i, j = i+1, j+1
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		first := 0
		for i < len(a) && j < len(b) && isDigit(a[i]) && isDigit(b[j]) {
			if first == 0 {
				first = cmp.Compare(a[i], b[j])
			}
			i, j = i+1, j+1
		}
		switch {
		case i < len(a) && isDigit(a[i]):
			return 1
		case j < len(b) && isDigit(b[j]):
			return -1
		case first != 0:
			return first
		}
	}
	return 0
}

// order is the weight of s's character at i in a run of non-digits: ~
// lowest, then the run's end (or a digit), then letters, then the rest.
func order(s string, i int) int {
	switch {
	case i >= len(s) || isDigit(s[i]):
		return 0
	case s[i] == '~':
		return -1
	case isLetter(s[i]):
		return int(s[i])
	default:
		return int(s[i]) + 256
	}
}

func isDigit(c byte) bool  { return '0' <= c && c <= '9' }
func isLetter(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' }
