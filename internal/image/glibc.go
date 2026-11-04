package image

import (
	"regexp"
	"strconv"
	"strings"
)

// glibcPattern is a glibc version as devvm records one: major.minor, as
// getconf GNU_LIBC_VERSION and the GLIBC_ versions programs need have it.
var glibcPattern = regexp.MustCompile(`^[0-9]{1,3}\.[0-9]{1,3}$`)

// ValidGlibc reports whether s is a glibc version as devvm records one: a
// --from base's (2.36) and the newest a tool image needs.
func ValidGlibc(s string) bool { return glibcPattern.MatchString(s) }

// GlibcOlder reports whether glibc version a is older than b, both
// ValidGlibc: 2.9 is older than 2.28.
func GlibcOlder(a, b string) bool {
	am, an := glibcParts(a)
	bm, bn := glibcParts(b)
	return am < bm || am == bm && an < bn
}

func glibcParts(v string) (major, minor int) {
	a, b, _ := strings.Cut(v, ".")
	major, _ = strconv.Atoi(a)
	minor, _ = strconv.Atoi(b)
	return major, minor
}
