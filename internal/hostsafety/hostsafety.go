// Package hostsafety checks a project folder, on the host, for what a VM
// that mounts it could leave there or can't do from it: set-id files,
// whose bits reach Linux hosts, and an origin remote over SSH, which a VM
// without an SSH agent can't reach. gitprotect, beside it, guards .git.
package hostsafety

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/hostgit"
	"github.com/w3cj/devvm/internal/platform"
)

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
// it: the user, or root. Every .git folder is skipped: nothing runs a file
// in one by its path, and .git holds most of a big project's files. The
// walk reads each file's metadata, a second or so for 200,000 files, so
// it runs once the VM is stopped and when a VM is made, never at a launch
// into a running VM.
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
			if path != dir && d.Name() == ".git" {
				return filepath.SkipDir
			}
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
