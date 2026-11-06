package gitprotect

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/devvmhq/devvm/internal/msg"
)

// OwnRepo takes a .git folder, or a linked worktree whose link git made
// both ways, and names whatever else points into another repo.
func TestOwnRepo(t *testing.T) {
	needGit(t)
	f := newFixture(t, "repo")
	root, repo := f.root, f.dir
	f.git(repo, "init", "-q")
	f.git(repo, "commit", "-q", "--allow-empty", "-m", "one")
	wt := filepath.Join(root, "wt")
	f.git(repo, "worktree", "add", "-q", "-b", "wt", wt)
	g, entry := filepath.Join(repo, ".git"), filepath.Join(repo, ".git", "worktrees", "wt")

	check := func(name, dir, want string) {
		t.Helper()
		gd, cd, err := OwnRepo(dir)
		got := msg.NamesOf(err)
		if err == nil {
			got = gd + " " + cd
		}
		if got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	// set writes data to path for one check, then puts back what was there.
	set := func(path, data string, fn func()) {
		t.Helper()
		had := os.Rename(path, path+".away") == nil
		write(t, path, data)
		fn()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if had {
			if err := os.Rename(path+".away", path); err != nil {
				t.Fatal(err)
			}
		}
	}
	// replaced swaps path for a symlink to target for one check.
	replaced := func(path, target string, fn func()) {
		t.Helper()
		if err := os.Rename(path, path+".away"); err != nil {
			t.Fatal(err)
		}
		symlink(t, target, path)
		fn()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".away", path); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(root, "other")
	f.git(root, "init", "-q", other)
	f.git(other, "commit", "-q", "--allow-empty", "-m", "one")
	og := filepath.Join(other, ".git")

	check("a repo", repo, g+" "+g)
	check("its worktree", wt, entry+" "+g)
	check("no .git", root, "clone_not_top dir="+root)

	set(filepath.Join(g, "commondir"), og+"\n", func() {
		check("a commondir", repo, "clone_git_elsewhere path="+g+"/commondir")
		check("its worktree, the repo's commondir elsewhere", wt, "clone_git_elsewhere path="+wt+"/.git")
	})
	set(filepath.Join(entry, "commondir"), og+"\n", func() {
		check("a worktree's commondir elsewhere", wt, "clone_git_elsewhere path="+wt+"/.git")
	})
	set(filepath.Join(entry, "gitdir"), "../../../../wt/.git\n", func() {
		check("a relative back-link", wt, entry+" "+g)
	})
	set(filepath.Join(wt, ".git"), "gitdir: "+filepath.Join(repo, ".git", "elsewhere", "wt")+"\n", func() {
		check("not in worktrees", wt, "clone_git_elsewhere path="+wt+"/.git")
	})
	set(filepath.Join(wt, ".git"), "nothing\n", func() {
		check("no gitdir line", wt, "clone_git_elsewhere path="+wt+"/.git")
	})
	set(filepath.Join(g, "objects", "info", "alternates"), og+"/objects\n", func() {
		check("alternates", repo, "clone_alternates path="+g+"/objects/info/alternates")
		check("alternates, from a worktree", wt, "clone_alternates path="+g+"/objects/info/alternates")
	})
	replaced(g, og, func() {
		check("a symlinked .git", repo, "clone_git_elsewhere path="+g)
	})
	replaced(filepath.Join(g, "objects"), filepath.Join(og, "objects"), func() {
		check("symlinked objects", repo, "clone_git_elsewhere path="+g+"/objects")
	})
	replaced(filepath.Join(g, "objects", "pack"), filepath.Join(og, "objects", "pack"), func() {
		check("a symlinked pack folder", repo, "clone_git_elsewhere path="+g+"/objects/pack")
	})
	loose := absentFanout(t, filepath.Join(g, "objects"))
	symlink(t, filepath.Join(og, "objects"), loose)
	check("a symlinked loose objects' folder", repo, "clone_git_elsewhere path="+loose)
	if err := os.Remove(loose); err != nil {
		t.Fatal(err)
	}
	replaced(filepath.Join(g, "refs", "heads"), filepath.Join(og, "refs", "heads"), func() {
		check("symlinked heads", repo, "clone_git_elsewhere path="+g+"/refs/heads")
	})
	symlink(t, filepath.Join(og, "packed-refs"), filepath.Join(g, "packed-refs"))
	check("symlinked packed refs", repo, "clone_git_elsewhere path="+g+"/packed-refs")
}

// absentFanout returns a loose objects' fan-out folder in objects that
// none of the repo's own objects use, so the test can make it.
func absentFanout(t *testing.T, objects string) string {
	t.Helper()
	for i := range 256 {
		p := filepath.Join(objects, fmt.Sprintf("%02x", i))
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return p
		}
	}
	t.Fatal("every fan-out folder exists")
	return ""
}
