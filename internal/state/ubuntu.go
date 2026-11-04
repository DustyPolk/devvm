package state

import (
	"regexp"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

// UbuntuPin is the Ubuntu cloud image base starts from on this machine,
// images/ubuntu.json: a dated release of Ubuntu's, its image's file for
// the host and that file's SHA-256, from the release's signed checksums.
// It's pinned at base's first build and again by devvm rebuild base, and
// base's key hashes it.
type UbuntuPin struct {
	Release string    `json:"release"` // release-20261016
	File    string    `json:"file"`    // ubuntu-26.04-server-cloudimg-arm64.img
	Digest  string    `json:"digest"`  // sha256:<64 hex digits>
	Pinned  time.Time `json:"pinned"`  // when it was pinned
}

var (
	pinRelease = regexp.MustCompile(`^release-[0-9]{8}$`)
	pinFile    = regexp.MustCompile(`^ubuntu-[0-9.]+-server-cloudimg-[a-z0-9]+\.img$`)
	pinDigest  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// valid reports whether p's fields have their shapes: they go into base's
// key and Lima's config.
func (p UbuntuPin) valid() bool {
	return pinRelease.MatchString(p.Release) && pinFile.MatchString(p.File) && pinDigest.MatchString(p.Digest)
}

// ReadUbuntuPin reads the pinned cloud image. None pinned is an error
// that matches fs.ErrNotExist, and one whose fields aren't a pin's is a
// CorruptError.
func ReadUbuntuPin(d paths.Dirs) (UbuntuPin, error) {
	var p UbuntuPin
	if err := readJSON(d.UbuntuPin(), &p); err != nil {
		return UbuntuPin{}, err
	}
	if !p.valid() {
		return UbuntuPin{}, &CorruptError{Path: d.UbuntuPin(), Reason: "not a pinned cloud image"}
	}
	return p, nil
}

// WriteUbuntuPin pins a cloud image, replacing the one pinned.
func WriteUbuntuPin(d paths.Dirs, p UbuntuPin) error {
	if !p.valid() {
		return &CorruptError{Path: d.UbuntuPin(), Reason: "not a pinned cloud image"}
	}
	return writeJSON(d.UbuntuPin(), p)
}
