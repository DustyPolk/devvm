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

// HashOf returns the hash a project VM's name ends with, which names its
// folder in devvm's data folder (paths.Dirs.VMDir), and whether name is
// one's (IsVM).
func HashOf(name string) (string, bool) {
	if !IsVM(name) {
		return "", false
	}
	return name[len(name)-HashLen:], true
}

// asideSuffix ends the name a project VM's old machine has while a remake
// makes a new one by the VM's name (Aside).
const asideSuffix = "-old"

// Aside returns the name a project VM's old machine is given while a
// remake makes the new one by the VM's name, devvm-<hash>-old, and whether
// name is a project VM's. It isn't a project VM's name itself (IsVM), so
// nothing takes the old machine for a VM, and it's the same length for
// every VM, the shortest a project VM's name can be plus two.
func Aside(name string) (string, bool) {
	h, ok := HashOf(name)
	if !ok {
		return "", false
	}
	return Prefix + h + asideSuffix, true
}

// AsideHash returns the hash of the project VM whose old machine name is,
// as Aside names it, and whether it's one's.
func AsideHash(name string) (string, bool) {
	h, ok := strings.CutPrefix(name, Prefix)
	if !ok {
		return "", false
	}
	h, ok = strings.CutSuffix(h, asideSuffix)
	if !ok || !IsHash(h) {
		return "", false
	}
	return h, true
}

// IsHash reports whether s is a project VM's hash, as HashOf returns it:
// HashLen lowercase hex digits.
func IsHash(s string) bool { return hash.MatchString(s) }

// hash matches a project VM's hash.
var hash = regexp.MustCompile(`^[0-9a-f]{` + strconv.Itoa(HashLen) + `}$`)

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
