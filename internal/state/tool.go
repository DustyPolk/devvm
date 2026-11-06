package state

import (
	"errors"
	"time"

	"github.com/devvmhq/devvm/internal/paths"
)

// ToolImage is a tool image's record, <file>.json beside it in the share:
// written once the image is moved in, so an image without one isn't
// built.
type ToolImage struct {
	SHA256 string `json:"sha256"` // the image's SHA-256, in hex, as it was moved in
	Libc   string `json:"libc"`   // the C library it's built for: gnu, or musl
	// Glibc is, for gnu, the newest glibc version its programs and
	// libraries need, as its build measured it: "" when they need none.
	Glibc string `json:"glibc,omitempty"`
	// Base is, for musl, the digest of the --from base it was built in,
	// which its key hashes; gnu images build in devvm's own base.
	Base    string    `json:"base,omitempty"`
	Format  string    `json:"format"`  // its backend's ToolFormat: "" for shared-tools/build.sh's own
	Backend string    `json:"backend"` // the backend that built it
	Built   time.Time `json:"built"`   // when it was moved in
	MadeBy  string    `json:"made_by"` // the version of the devvm that built it
	// IdleSince is when retention first found it current with no VM:
	// zero while a VM has it.
	IdleSince time.Time `json:"idle_since,omitzero"`
}

// ReadToolImage reads the record of the tool image file in the share. A
// missing record is an error that matches fs.ErrNotExist.
func ReadToolImage(d paths.Dirs, file string) (ToolImage, error) {
	if err := checkName("tool image", file); err != nil {
		return ToolImage{}, err
	}
	path := d.ToolRecord(file)
	var t ToolImage
	if err := readJSON(path, &t); err != nil {
		return ToolImage{}, err
	}
	if t.SHA256 == "" {
		return ToolImage{}, &CorruptError{Path: path, Reason: "no sha256"}
	}
	return t, nil
}

// WriteToolImage writes the record of the tool image file in the share.
func WriteToolImage(d paths.Dirs, file string, t ToolImage) error {
	if err := checkName("tool image", file); err != nil {
		return err
	}
	if t.SHA256 == "" {
		return errors.New("a tool image record needs its SHA-256")
	}
	return writeJSON(d.ToolRecord(file), t)
}
