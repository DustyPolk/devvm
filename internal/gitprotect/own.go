package gitprotect

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// OwnRepo checks that what git reads in dir, the top folder of a repo,
// is that repo's own, for --clone, whose push sends it into a VM. dir's
// .git is a folder, or the .git file of a linked worktree whose link git
// made both ways (linkedWorktree); and nothing in its git dir or common
// dir points into another repo: no commondir in a .git folder, no
// alternates, and no objects or refs that are symlinks. It returns the
// git dir and the common dir, resolved (the same for a .git folder), or
// the message naming what points elsewhere.
func OwnRepo(dir string) (gitDir, commonDir string, err error) {
	g := filepath.Join(dir, ".git")
	fi, err := os.Lstat(g)
	switch {
	case err != nil:
		return "", "", msg.CloneNotTop(dir)
	case fi.IsDir():
		gitDir, _ = filepath.EvalSymlinks(g)
		if c, ok := commonOf(g); !ok || c != gitDir {
			return "", "", msg.CloneGitElsewhere(filepath.Join(g, "commondir"))
		}
		commonDir = gitDir
	case fi.Mode().IsRegular():
		if gitDir, commonDir = linkedWorktree(dir); gitDir == "" {
			return "", "", msg.CloneGitElsewhere(g)
		}
	default:
		return "", "", msg.CloneGitElsewhere(g)
	}
	for _, d := range []string{gitDir, commonDir} {
		if err := ownObjects(d); err != nil {
			return "", "", err
		}
	}
	return gitDir, commonDir, nil
}

// linkedWorktree returns the git dir and common dir of the linked
// worktree dir, resolved, when git made its link both ways: dir's .git
// file names <repo>/.git/worktrees/<name>, whose gitdir file names dir's
// .git back, and whose commondir leads to that <repo>/.git, a folder with
// no commondir of its own. Else it returns "", "".
func linkedWorktree(dir string) (gitDir, commonDir string) {
	g, err := filepath.EvalSymlinks(filepath.Join(dir, ".git"))
	if err != nil {
		return "", ""
	}
	t := gitdirOf(g, dir)
	repo := filepath.Dir(filepath.Dir(t))
	if t == "" || filepath.Base(filepath.Dir(t)) != "worktrees" || filepath.Base(repo) != ".git" {
		return "", ""
	}
	back, err := os.ReadFile(filepath.Join(t, "gitdir")) //nolint:gosec // the repo's own
	if err != nil {
		return "", ""
	}
	b := strings.TrimRight(string(back), "\n")
	if !filepath.IsAbs(b) {
		b = filepath.Join(t, b)
	}
	if r, err := filepath.EvalSymlinks(b); err != nil || r != g {
		return "", ""
	}
	if fi, err := os.Lstat(repo); err != nil || !fi.IsDir() {
		return "", ""
	}
	if c, ok := commonOf(t); !ok || c != repo {
		return "", ""
	}
	if c, ok := commonOf(repo); !ok || c != repo {
		return "", ""
	}
	return t, repo
}

// ownObjects refuses a git dir d that reads objects or refs from
// elsewhere: alternates, or objects, packed refs, a reftable or any of
// refs that's a symlink.
func ownObjects(d string) error {
	alt := filepath.Join(d, "objects", "info", "alternates")
	if _, err := os.Lstat(alt); !errors.Is(err, fs.ErrNotExist) {
		return msg.CloneAlternates(alt)
	}
	for _, name := range []string{"objects", "packed-refs", "reftable", "refs"} {
		if p := filepath.Join(d, name); isLink(p) {
			return msg.CloneGitElsewhere(p)
		}
	}
	var link string
	_ = filepath.WalkDir(filepath.Join(d, "refs"), func(path string, e fs.DirEntry, err error) error {
		if err == nil && e.Type()&fs.ModeSymlink != 0 {
			link = path
			return filepath.SkipAll
		}
		return nil
	})
	if link != "" {
		return msg.CloneGitElsewhere(link)
	}
	return nil
}
