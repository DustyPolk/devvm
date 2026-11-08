// Package hostsafety checks a project folder, on the host, for what a VM
// that mounts it could leave there or can't do from it, in one walk of
// the folder (Walk): set-id files, whose bits reach Linux hosts, and the
// git repos nested in it, which gitprotect, beside it, guards; and an
// origin remote over SSH, which a VM without an SSH agent can't reach.
package hostsafety

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/devvmhq/devvm/internal/hostgit"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/safefile"
)

// setIDMax is how many set-id files Walk returns at most.
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

// The walk's bounds for repos: it looks for them at most RepoDepth
// folders below the project folder, and returns at most MaxRepos of them,
// counting the rest and keeping the names of the first MaxPast of those.
const (
	RepoDepth = 4
	MaxRepos  = 100
	MaxPast   = 10
)

// Found is what one walk of a project folder found (Walk): its set-id
// files, when asked for, and the git repos nested in it.
type Found struct {
	SetID []SetIDFile
	Repos Repos
}

// Repos are the git repos nested in a project folder, below its own: each
// folder with anything named .git (IsGitName), a folder, a file, which a
// linked worktree or a separate git dir has, a symlink or anything else,
// as the VM can make any of them into one git takes, by its path relative
// to the project folder, in the walk's order, at most MaxRepos.
type Repos struct {
	Dirs []string
	More int      // how many more it found
	Past []string // the first MaxPast of those, to name
	// Bare is the folders shaped like a git dir (IsGitDir) that aren't a
	// .git, at most MaxRepos: git run in a folder below one can take it
	// for its repo, a bare repo whose config can name a work tree.
	Bare []string
	// Skipped is how many .git the walk saw where it doesn't look for
	// repos (deeper than RepoDepth, or in noRepos), which it sees only
	// where it walks for set-id files, and Devices how many folders it
	// didn't enter as they're on another file system.
	Skipped int
	Devices int
}

// IsGitName reports whether name is .git as git finds it here: whatever
// its case on a file system that folds case (platform.FoldsCase), where
// .GIT is .git.
func IsGitName(name string) bool {
	return name == ".git" || platform.FoldsCase && strings.EqualFold(name, ".git")
}

// Walk walks dir once, on its file system, following no symlink, for
// what a VM that mounts it could leave there: the git repos nested in it
// (Repos), and with setID, up to five set-id files. On Linux hosts a VM's
// set-id bits reach the host, and a set-user-id file runs as its owner
// for anyone who runs it: the user, or root. Every .git folder is
// skipped: nothing runs a file in one by its path, git's own repos in it
// (submodules') are gitprotect's to find, and .git holds most of a big
// project's files. Repos are looked for at most RepoDepth folders down,
// and not in the folders of packages and build tools (noRepos), which
// rarely hold a repo anyone runs git in, so a walk without setID stops
// there. With it, the walk reads each file's metadata, a second or so for
// 200,000 files, so it runs once the VM is stopped and when a VM is made,
// never at a launch into a running VM.
func Walk(dir string, setID bool) Found {
	root, err := os.Stat(dir)
	if err != nil {
		return Found{}
	}
	w := walker{dir: dir, root: root, setID: setID}
	_ = filepath.WalkDir(dir, w.visit)
	return w.found
}

// walker is one Walk's state.
type walker struct {
	dir   string
	root  fs.FileInfo
	setID bool // set-id files are still to find
	found Found
}

func (w *walker) visit(path string, d fs.DirEntry, err error) error {
	if err != nil || path == w.dir {
		return nil //nolint:nilerr // an unreadable folder: skip it, like find
	}
	rel := strings.TrimPrefix(path, w.dir+"/")
	if IsGitName(d.Name()) {
		w.repo(rel)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if d.IsDir() {
		return w.folder(path, rel, d)
	}
	if w.setID && d.Type().IsRegular() {
		w.setIDFile(path, d)
	}
	return nil
}

// folder is visit's for a folder: one on another file system is counted
// and not entered, one shaped like a git dir is named (Repos.Bare), and
// with no set-id files to find, one where no repo is looked for isn't
// entered.
func (w *walker) folder(path, rel string, d fs.DirEntry) error {
	r := &w.found.Repos
	if fi, err := d.Info(); err == nil && !platform.SameDevice(w.root, fi) {
		r.Devices++
		return filepath.SkipDir
	}
	if reposIn(rel) && len(r.Bare) < MaxRepos && IsGitDir(path) {
		r.Bare = append(r.Bare, rel)
	}
	if !w.setID && !reposIn(rel) {
		return filepath.SkipDir
	}
	return nil
}

// repo records the folder of a .git at rel, whatever it is, when the walk
// looks for repos there, but not the project folder's own: git takes a
// .git that isn't a repo's yet for none and looks above it, but the VM can
// make it one at any time.
func (w *walker) repo(rel string) {
	folder := filepath.Dir(rel)
	r := &w.found.Repos
	switch {
	case folder == ".":
	case !reposIn(folder):
		r.Skipped++
	case len(r.Dirs) < MaxRepos:
		r.Dirs = append(r.Dirs, folder)
	default:
		r.More++
		if len(r.Past) < MaxPast {
			r.Past = append(r.Past, folder)
		}
	}
}

// reposIn reports whether the walk looks for a repo's .git in the folder
// rel, relative to the project folder: it's at most RepoDepth folders
// down, and neither it nor a folder above it is one of noRepos.
func reposIn(rel string) bool {
	parts := strings.Split(rel, "/")
	if len(parts) > RepoDepth {
		return false
	}
	return !slices.ContainsFunc(parts, noRepos)
}

// noRepos reports whether a folder is one the walk doesn't look for repos
// in, for speed: packages and build tools' own, which they fill with
// what they fetch.
func noRepos(name string) bool {
	switch name {
	case "node_modules", "vendor", ".venv", "__pycache__", ".terraform":
		return true
	}
	return false
}

// setIDFile records the file at path when it has a set-id bit, up to
// setIDMax.
func (w *walker) setIDFile(path string, d fs.DirEntry) {
	fi, err := d.Info()
	if err != nil || fi.Mode()&(fs.ModeSetuid|fs.ModeSetgid) == 0 {
		return
	}
	uid, gid, ok := platform.Owner(fi)
	w.found.SetID = append(w.found.SetID, SetIDFile{Path: path, Root: ok && rootSetID(fi.Mode(), uid, gid)})
	if len(w.found.SetID) == setIDMax {
		w.setID = false
	}
}

// IsGitDir reports whether dir is what git could take for a git dir, as
// git's own is_git_directory has it: a HEAD (a file, or a symlink) and
// objects and refs, files or folders, in its common dir, the folder its
// commondir file names (relative to dir), or else dir itself. It takes
// more than git would, never less: git also checks HEAD's contents and
// that objects and refs can be searched, which the VM can change at any
// time. A .git file can name any folder, so gitprotect locks the hooks
// and config only of one that passes.
func IsGitDir(dir string) bool {
	fi, err := os.Lstat(filepath.Join(dir, "HEAD"))
	if err != nil || !fi.Mode().IsRegular() && fi.Mode()&fs.ModeSymlink == 0 {
		return false
	}
	common := dir
	if c, err := safefile.Read(filepath.Join(dir, "commondir"), maxCommondir, safefile.Opts{Follow: true}); err == nil {
		common = strings.TrimRight(string(c), "\n")
		if !filepath.IsAbs(common) {
			common = filepath.Join(dir, common)
		}
	}
	return exists(filepath.Join(common, "objects")) && exists(filepath.Join(common, "refs"))
}

// maxCommondir is the most of a commondir file IsGitDir reads.
const maxCommondir = 4096

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// BareExplicit reports whether git on this machine takes a bare repo only
// when it's named (safe.bareRepository=explicit, git 2.38 and newer), in
// your global config or the system's, the files GIT_CONFIG_GLOBAL and
// GIT_CONFIG_SYSTEM name included (hostgit.UserConfig): else git run in
// a folder below one shaped like a git dir, which a VM can make in its
// folder, takes it for its repo, config and all. It only reads.
func BareExplicit(ctx context.Context) bool {
	v, _ := hostgit.UserConfig(ctx, "safe.bareRepository")
	return strings.EqualFold(strings.TrimSpace(v), "explicit")
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
