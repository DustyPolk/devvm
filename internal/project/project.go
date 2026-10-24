// Package project is a project folder as devvm sees it: its path and its
// VM's name, the folders a VM must never get read-write, the toolchains its
// files call for, and what in it a VM could turn against the host.
package project

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // names, not security: 56 bits tell folders apart
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/w3cj/devvm/internal/platform"
)

// Prefix starts every project VM's name: devvm-<folder>-<hash>.
const Prefix = "devvm-"

// hashLen is how many hex digits of the path's hash a VM's name has: 56
// bits, so another folder with the same name is out of reach, and the
// length tells project VMs apart from images (devvm-img-<12>).
const hashLen = 14

// slugMax is the most of the folder's name a VM's name has.
const slugMax = 20

// slugMin is the least a VM's name needs of the folder's name. A backend
// whose names leave less room is refused.
const slugMin = 4

// ErrNoRoom is New's error when the backend's VM names leave no room for
// the folder's name.
var ErrNoRoom = errors.New("no room for the folder's name in the VM's name")

// Folder is a project folder and its VM's name.
type Folder struct {
	Dir  string // its physical path, as pwd -P has it
	Name string // its VM's name: devvm-<folder>-<hash of Dir>
}

// BadPathError is a folder whose path Lima can't mount: it has a comma
// (Lima splits mount options on commas) or a control character.
type BadPathError struct{ Dir string }

func (e *BadPathError) Error() string { return "can't mount " + e.Dir }

// New returns the project folder at dir, a physical path, and its VM's
// name, fitting nameMax characters (0 for no limit).
func New(dir string, nameMax int) (Folder, error) {
	if strings.Contains(dir, ",") || strings.IndexFunc(dir, unicode.IsControl) >= 0 {
		return Folder{}, &BadPathError{Dir: dir}
	}
	room := slugMax
	if nameMax > 0 {
		room = min(room, nameMax-len(Prefix)-1-hashLen)
	}
	if room < slugMin {
		return Folder{}, ErrNoRoom
	}
	sum := sha1.Sum([]byte(dir)) //nolint:gosec // see the import
	return Folder{Dir: dir, Name: Prefix + Slug(filepath.Base(dir), room) + "-" + hex.EncodeToString(sum[:])[:hashLen]}, nil
}

// Slug is a folder's name as it goes into its VM's name: lowercase, each
// character but a-z and 0-9 as a dash, without dashes at either end, at
// most n characters (root when nothing's left).
func Slug(base string, n int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "root"
	}
	if len(s) > n {
		s = strings.TrimRight(s[:n], "-")
	}
	return s
}

// Unsafe is what a project folder mustn't be, hold or be inside: the user's
// home, devvm's config and data folders and its Lima home. A VM gets its
// folder read-write.
type Unsafe struct {
	Home, Config, Data, Lima string
}

// Holds returns the first of these folders that dir is or holds (any
// folder holds /), or "". The VM would get them read-write: tokens, the
// proxy's CA key and binary, other VMs' disks.
func (u Unsafe) Holds(dir string) string {
	for _, p := range []string{u.Home, u.Config, u.Data, u.Lima} {
		if r := physical(p); r == dir || strings.HasPrefix(r, dir+"/") || dir == "/" {
			return p
		}
	}
	return ""
}

// Inside returns the first of devvm's own folders that dir is inside, or
// "". A VM in the layers folder could rewrite the scripts every image build
// runs; one in the data folder, the binaries that run on the host.
func (u Unsafe) Inside(dir string) string {
	for _, p := range []string{u.Config, u.Data, u.Lima} {
		if strings.HasPrefix(dir, physical(p)+"/") {
			return p
		}
	}
	return ""
}

// physical returns path with its symlinks resolved, as far as it exists:
// a folder devvm hasn't made yet is where it will be.
func physical(path string) string {
	rest := ""
	for p := filepath.Clean(path); ; p = filepath.Dir(p) {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		if filepath.Dir(p) == p {
			return path
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

// markers are the files in a folder's top level that call for each
// toolchain, in the order Detect lists them: the same mix always gives the
// same image.
var markers = []struct {
	toolchain string
	files     []string
}{
	{"node", []string{"package.json", "bun.lock", "bun.lockb"}},
	{"python", []string{"pyproject.toml", "requirements.txt", "uv.lock", "setup.py", "Pipfile"}},
	{"rust", []string{"Cargo.toml"}},
	{"go", []string{"go.mod"}},
}

// Detect returns the toolchains whose marker files are in dir's top level,
// in a fixed order.
func Detect(dir string) []string {
	var found []string
	for _, m := range markers {
		for _, f := range m.files {
			if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
				found = append(found, m.toolchain)
				break
			}
		}
	}
	return found
}

// setIDMax is how many set-id files SetIDFiles returns at most.
const setIDMax = 5

// SetIDFiles returns up to five files in dir, on its file system, with the
// set-user-id or set-group-id bit. On Linux hosts a VM's set-id bits reach
// the host, and the files are the user's: a set-user-id one runs as the
// user for anyone who runs it.
func SetIDFiles(dir string) []string {
	var files []string
	root, err := os.Stat(dir)
	if err != nil {
		return nil
	}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable folder: skip it, like find
		}
		if d.IsDir() {
			if fi, err := d.Info(); err == nil && path != dir && !platform.SameDevice(root, fi) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err == nil && fi.Mode()&(fs.ModeSetuid|fs.ModeSetgid) != 0 {
			files = append(files, path)
		}
		if len(files) == setIDMax {
			return filepath.SkipAll
		}
		return nil
	})
	return files
}

// SSHRemote reports whether dir's repo fetches origin over SSH, which a VM
// without an SSH agent can't.
func SSHRemote(ctx context.Context, dir string) bool {
	out, err := exec.CommandContext(ctx, "git", "config", "--file", //nolint:gosec // devvm's own arguments
		filepath.Join(dir, ".git", "config"), "--get", "remote.origin.url").Output()
	if err != nil {
		return false
	}
	s := bufio.NewScanner(strings.NewReader(string(out)))
	if !s.Scan() {
		return false
	}
	url := s.Text()
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}
