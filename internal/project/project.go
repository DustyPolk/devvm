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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/w3cj/devvm/internal/hostgit"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/vmname"
)

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

// Slug is the folder's part of its VM's name, which is also the VM's
// hostname (vmname.SlugOf).
func (f Folder) Slug() string {
	s, _ := vmname.SlugOf(f.Name)
	return s
}

// Hash is the hash of dir, a physical path, that its VM's name ends with
// (vmname.HashOf): 56 bits of its SHA-1, in hex. It names the VM's folder
// in devvm's data folder, so a folder's VM is found by its path alone,
// whatever name its backend gave it.
func Hash(dir string) string {
	sum := sha1.Sum([]byte(dir)) //nolint:gosec // see the import
	return hex.EncodeToString(sum[:])[:vmname.HashLen]
}

// IsPath reports whether a link's TARGET is a folder's path: it has a / or
// starts with a dot.
func IsPath(target string) bool {
	return strings.Contains(target, "/") || strings.HasPrefix(target, ".")
}

// ForDir returns the project VMs among all (VM names) made for dir, a
// physical path: those whose name ends with its path's hash, whatever
// length of its folder's name their backend gave them.
func ForDir(dir string, all []string) []string {
	var out []string
	for _, vm := range all {
		if h, ok := vmname.HashOf(vm); ok && h == Hash(dir) {
			out = append(out, vm)
		}
	}
	return out
}

// Recorded is a project VM as its state records it: its name, the alias
// ssh reaches it by, and its project folder.
type Recorded struct{ Name, Alias, Dir string }

// Named returns the project VM among vms whose name or alias is name, ""
// when none is.
func Named(name string, vms []Recorded) string {
	for _, v := range vms {
		if vmname.IsVM(v.Name) && (name == v.Name || name == v.Alias) {
			return v.Name
		}
	}
	return ""
}

// Targets returns the project VMs among vms that a link's TARGET names
// when it isn't a path (IsPath), in vms's order: the VM itself for a VM's
// name or alias, else those whose folder's name has the same Slug, as New
// makes it, whatever their backend left of it in their names.
func Targets(target string, vms []Recorded) []string {
	if vm := Named(target, vms); vm != "" {
		return []string{vm}
	}
	if vmname.IsVM(target) {
		return nil
	}
	want := Slug(target, slugMax)
	var out []string
	for _, v := range vms {
		slug := Folder{Name: v.Name}.Slug()
		if v.Dir != "" {
			slug = Slug(filepath.Base(v.Dir), slugMax)
		}
		if vmname.IsVM(v.Name) && slug == want {
			out = append(out, v.Name)
		}
	}
	return out
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
		room = min(room, nameMax-len(vmname.Prefix)-1-vmname.HashLen)
	}
	if room < slugMin {
		return Folder{}, ErrNoRoom
	}
	return Folder{Dir: dir, Name: vmname.Prefix + Slug(filepath.Base(dir), room) + "-" + Hash(dir)}, nil
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

// Parents returns the folders dir, a physical path, is inside, nearest
// first, whose VM a command in dir may use: up to the home folder (home,
// resolved) and the root, both left out, as is everything above home
// when dir is home or in it.
func Parents(dir, home string) []string {
	home = physical(home)
	if dir == home {
		return nil
	}
	var ps []string
	for p := filepath.Dir(dir); p != home && filepath.Dir(p) != p; p = filepath.Dir(p) {
		ps = append(ps, p)
	}
	return ps
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

// nixMarkers are a Nix project's own files, which call for the nix layer.
var nixMarkers = []string{"flake.nix", "shell.nix", "devenv.nix"}

// marker is a part and the files in a folder's top level that call for it.
type marker struct {
	part  string
	files []string
}

// markers are the files that call for each toolchain (image.SharedTools'
// Detect), then for the nix layer, in the order DetectFiles lists them:
// the same mix always gives the same image.
func markers() []marker {
	var ms []marker
	for _, d := range image.SharedTools {
		if len(d.Detect) > 0 {
			ms = append(ms, marker{d.Name, d.Detect})
		}
	}
	return append(ms, marker{image.NixLayer, nixMarkers})
}

// nixShellMax is how much of a default.nix nixShell reads.
const nixShellMax = 64 << 10

// nixShell reports whether dir's default.nix makes a shell (mkShell, or
// mkShellNoCC), as one for nix-shell does; most build a package instead.
func nixShell(dir string) bool {
	p := filepath.Join(dir, "default.nix")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(p) //nolint:gosec // a regular file in the project folder, read
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(io.LimitReader(f, nixShellMax))
	return strings.Contains(string(data), "mkShell")
}

// Found is a toolchain, or the nix layer, that a folder calls for, and the
// first of its marker files that calls for it.
type Found struct{ Name, File string }

// DetectFiles returns the toolchains and the nix layer whose marker files
// are in dir's top level, in a fixed order, each with the file that calls
// for it: nix also for a default.nix that makes a shell.
func DetectFiles(dir string) []Found {
	var found []Found
	for _, m := range markers() {
		file := ""
		for _, f := range m.files {
			if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
				file = f
				break
			}
		}
		if file == "" && m.part == image.NixLayer && nixShell(dir) {
			file = "default.nix"
		}
		if file != "" {
			found = append(found, Found{m.part, file})
		}
	}
	return found
}

// setIDMax is how many set-id files SetIDFiles returns at most.
const setIDMax = 5

// SetIDFile is a file with the set-user-id or set-group-id bit; Root when
// it runs as root or with root's group (rootSetID).
type SetIDFile struct {
	Path string
	Root bool
}

// rootSetID reports whether a file with mode m, of uid and gid, runs as
// root (set-user-id, root's) or with root's group (set-group-id, its).
func rootSetID(m fs.FileMode, uid, gid uint32) bool {
	return m&fs.ModeSetuid != 0 && uid == 0 || m&fs.ModeSetgid != 0 && gid == 0
}

// SetIDFiles returns up to five files in dir, on its file system, with the
// set-user-id or set-group-id bit. On Linux hosts a VM's set-id bits reach
// the host, and a set-user-id file runs as its owner for anyone who runs
// it: the user, or root.
func SetIDFiles(dir string) []SetIDFile {
	var files []SetIDFile
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
			uid, gid, ok := platform.Owner(fi)
			files = append(files, SetIDFile{Path: path, Root: ok && rootSetID(fi.Mode(), uid, gid)})
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
	out, err := hostgit.Output(ctx, "config", "--file", filepath.Join(dir, ".git", "config"), "--get", "remote.origin.url")
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
