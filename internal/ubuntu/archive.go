package ubuntu

import (
	"errors"
	"io/fs"
	"regexp"
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

// ErrRelease is an InRelease that isn't one of Series' suites, or whose
// fields can't be read.
var ErrRelease = errors.New("not an InRelease of Ubuntu " + Version)

// Release is a suite's InRelease, its signature checked.
type Release struct {
	Suite string    // resolute, resolute-updates…
	Date  time.Time // when Ubuntu signed it
	lists map[string]int64
}

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

var listSum = regexp.MustCompile(`^ ([0-9a-f]{64}) +([0-9]+) +([a-z0-9-]+/binary-[a-z0-9]+/Packages)$`)

// readRelease reads a verified InRelease's text: its Suite, its Date and,
// of its SHA256 field, each component's uncompressed Packages list.
func readRelease(text string) (Release, error) {
	r := Release{lists: map[string]int64{}}
	field := ""
	for line := range strings.Lines(text) {
		line = strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(line, " ") {
			if m := listSum.FindStringSubmatch(line); field == "SHA256" && m != nil {
				n, err := strconv.ParseInt(m[2], 10, 64)
				if err != nil {
					return Release{}, ErrRelease
				}
				r.lists[m[1]] = n
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
	if (r.Suite != Series && !strings.HasPrefix(r.Suite, Series+"-")) || r.Date.IsZero() {
		return Release{}, ErrRelease
	}
	return r, nil
}

// Lists reports whether r lists a package list with SHA-256 sum and size.
func (r Release) Lists(sum string, size int64) bool {
	n, ok := r.lists[sum]
	return ok && n == size
}
