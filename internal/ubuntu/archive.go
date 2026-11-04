package ubuntu

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Ubuntu's archive, as base's recorded packages need it: a suite's
// InRelease, clearsigned by Ubuntu's archive key, which lists each of its
// package lists with its SHA-256 and size. A list whose SHA-256 the
// signed InRelease gives is the archive's, whoever served it, and so is
// each package whose SHA-256 that list gives.

// ArchiveKeyring is Ubuntu's archive signing key, by its path in devvm's
// own files: "Ubuntu Archive Automatic Signing Key (2018)
// <ftpmaster@ubuntu.com>", fingerprint
// F6ECB3762474EDA9D21B7022871920D1991BC93C, fetched from
// keyserver.ubuntu.com by that fingerprint and checked against the one
// Ubuntu's security team lists (wiki.ubuntu.com/SecurityTeam/FAQ, GPG keys
// used by Ubuntu), exported with gpg --export-options export-minimal.
const ArchiveKeyring = "layers/ubuntu-archive.gpg"

// Released is the day Series was released, when Ubuntu stopped changing
// its release pocket: that suite's InRelease is dated then, and one dated
// before it is a development state of the archive.
const Released = "20260423"

// pocketLag is how much older than the pinned cloud image's release a
// pocket that keeps changing (-updates, -security, -backports) may be:
// the image was built from the archive as it was that day, and a pocket
// last published the day before is as new.
const pocketLag = 24 * time.Hour

// ErrRelease is an InRelease that isn't one of Series' suites, or whose
// fields can't be read.
var ErrRelease = errors.New("not an InRelease of Ubuntu " + Version)

// Release is a suite's InRelease, its signature checked.
type Release struct {
	Suite string    // resolute, resolute-updates…
	Date  time.Time // when Ubuntu signed it
	lists []List
}

// List is one of a suite's uncompressed package lists, as its InRelease
// gives it.
type List struct {
	Path   string // main/binary-arm64/Packages
	SHA256 string
	Size   int64
}

// Arch is the architecture l lists the packages of, as its path names it
// (main/binary-arm64/Packages: arm64).
func (l List) Arch() string {
	_, dir, _ := strings.Cut(path.Dir(l.Path), "/binary-")
	return dir
}

// RequiredSuites are the pockets whose lists every recording of base's
// packages must give back: the ones that hold newer versions than the
// release pocket's, which apt installs instead.
func RequiredSuites() []string {
	return []string{Series + "-updates", Series + "-security"}
}

// Backports reports whether r is the backports pocket, whose packages apt
// installs only when asked to (Ubuntu signs it NotAutomatic), so a newer
// version there is no newer one apt would choose.
func (r Release) Backports() bool { return r.Suite == Series+"-backports" }

// VerifyRelease checks that inRelease is clearsigned by Ubuntu's archive
// key (ArchiveKeyring, in assets, devvm's own files) at now and reads what
// it signed. It fails for a signature that doesn't check out, and with
// ErrRelease for a suite of another series or fields that aren't an
// InRelease's.
func VerifyRelease(assets fs.FS, inRelease []byte, now time.Time) (Release, error) {
	keyring, err := fs.ReadFile(assets, ArchiveKeyring)
	if err != nil {
		return Release{}, err
	}
	text, err := verifyClearsigned(keyring, inRelease, now)
	if err != nil {
		return Release{}, err
	}
	return readRelease(string(text))
}

// knownSuite reports whether s is a suite an Ubuntu image's sources name:
// the release pocket and the pockets Ubuntu publishes to it, not
// -proposed's untested packages.
func knownSuite(s string) bool {
	return slices.Contains([]string{Series, Series + "-updates", Series + "-security", Series + "-backports"}, s)
}

var listSum = regexp.MustCompile(`^ ([0-9a-f]{64}) +([0-9]+) +([a-z0-9-]+/binary-[a-z0-9]+/Packages)$`)

// readRelease reads a verified InRelease's text: its Suite, its Date and,
// of its SHA256 field, each component's uncompressed Packages list.
func readRelease(text string) (Release, error) {
	var r Release
	field := ""
	for line := range strings.Lines(text) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, " ") {
			if m := listSum.FindStringSubmatch(line); field == "SHA256" && m != nil {
				n, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					return Release{}, ErrRelease
				}
				r.lists = append(r.lists, List{Path: m[3], SHA256: m[1], Size: n})
			}
			continue
		}
		k, v, _ := strings.Cut(line, ":")
		field, v = k, strings.TrimSpace(v)
		switch k {
		case "Suite":
			r.Suite = v
		case "Date":
			t, err := time.Parse(time.RFC1123, v)
			if err != nil {
				if t, err = time.Parse(time.RFC1123Z, v); err != nil {
					return Release{}, ErrRelease
				}
			}
			r.Date = t.UTC()
		}
	}
	if !knownSuite(r.Suite) || r.Date.IsZero() {
		return Release{}, ErrRelease
	}
	return r, nil
}

// Lists is each package list r gives with SHA-256 sum and size: none for
// a list that isn't r's, and more than one when lists are the same, as
// empty ones are.
func (r Release) Lists(sum string, size int64) []List {
	var out []List
	for _, l := range r.lists {
		if l.SHA256 == sum && l.Size == size {
			out = append(out, l)
		}
	}
	return out
}

// ListsFor is every package list r gives for arch's packages.
func (r Release) ListsFor(arch string) []List {
	var out []List
	for _, l := range r.lists {
		if l.Arch() == arch {
			out = append(out, l)
		}
	}
	return out
}

// Stale reports whether r is an older state of the archive than the
// pinned cloud image's release (release-YYYYMMDD) was built from: the
// release pocket dated before Released, or any other pocket dated more
// than a day before release. Ubuntu's InRelease files carry no
// Valid-Until, so a mirror that stopped updating still serves validly
// signed ones. A release that isn't a date makes every other pocket
// stale.
func (r Release) Stale(release string) bool {
	if r.Suite == Series {
		return r.Date.Before(day(Released))
	}
	d := day(strings.TrimPrefix(release, "release-"))
	return d.IsZero() || r.Date.Before(d.Add(-pocketLag))
}

// day is YYYYMMDD's start, in UTC; zero when it isn't a date.
func day(s string) time.Time {
	t, err := time.Parse("20060102", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
