// Package vmname is the grammar of a project VM's name,
// devvm-<folder>-<hash>: what starts it, how long its hash is, whether a
// name is one, and its folder part. Standard library only, so the
// backends read names as internal/project makes them.
package vmname

import (
	"regexp"
	"strconv"
	"strings"
)

// Prefix starts every project VM's name: devvm-<folder>-<hash>.
const Prefix = "devvm-"

// HashLen is how many hex digits of its folder's path's hash a project
// VM's name ends with: 56 bits, so another folder with the same name is
// out of reach, and the length tells project VMs apart from images
// (devvm-img-<12>).
const HashLen = 14

// vm matches a project VM's name.
var vm = regexp.MustCompile(`^` + regexp.QuoteMeta(Prefix) + `.+-[0-9a-f]{` + strconv.Itoa(HashLen) + `}$`)

// IsVM reports whether name is a project VM's: devvm-<folder>-<hash>.
// Images and build VMs aren't, a folder named img-… included
// (devvm-img-…-<hash>).
func IsVM(name string) bool { return vm.MatchString(name) }

// SlugOf returns the folder part of a name made as devvm-<folder>-<hash>,
// which is also a project VM's hostname, and whether it has one: the name
// starts with Prefix, and a dash follows a folder part that isn't empty.
// It reads build VMs' names alike (devvm-img-<key> gives img).
func SlugOf(name string) (string, bool) {
	s, ok := strings.CutPrefix(name, Prefix)
	if i := strings.LastIndexByte(s, '-'); ok && i > 0 {
		return s[:i], true
	}
	return "", false
}
