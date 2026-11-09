//go:build unix

package platform

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// noLinksTree is a tree for OpenNoLinks's tests, under root, with
// must failing the test on an error.
type noLinksTree struct {
	t    *testing.T
	root string
}

func newNoLinksTree(t *testing.T) noLinksTree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return noLinksTree{t, root}
}

func (n noLinksTree) must(err error) {
	n.t.Helper()
	if err != nil {
		n.t.Fatal(err)
	}
}

func (n noLinksTree) at(parts ...string) string {
	return filepath.Join(append([]string{n.root}, parts...)...)
}

func (n noLinksTree) file(content string, parts ...string) {
	n.t.Helper()
	n.must(os.MkdirAll(filepath.Dir(n.at(parts...)), 0o700))
	n.must(os.WriteFile(n.at(parts...), []byte(content), 0o600))
}

func (n noLinksTree) link(target string, parts ...string) {
	n.t.Helper()
	n.must(os.MkdirAll(filepath.Dir(n.at(parts...)), 0o700))
	n.must(os.Symlink(target, n.at(parts...)))
}

// following follows the links at the paths given and no other, as
// OpenNoLinks follows root's links and no other.
func (n noLinksTree) following(paths ...string) func(*unix.Stat_t) bool {
	n.t.Helper()
	inos := map[uint64]bool{}
	for _, p := range paths {
		var st unix.Stat_t
		n.must(unix.Lstat(p, &st))
		inos[st.Ino] = true
	}
	return func(st *unix.Stat_t) bool { return inos[st.Ino] }
}

// readOf is what opening path with follow reads.
func readOf(t *testing.T, path string, follow func(*unix.Stat_t) bool) (string, error) {
	t.Helper()
	f, err := openNoLinks(path, follow)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	return string(b), err
}

// OpenNoLinks reads a file whose path has no link, and refuses one with a
// link at its end, one with a link of the user's in a folder on the way,
// even when the link was swapped in after the path was resolved, a
// relative path and a folder.
func TestOpenNoLinks(t *testing.T) {
	n := newNoLinksTree(t)
	n.file("png", "project", "shots", "shot.png")
	n.file("secret", "elsewhere", "shots", "shot.png")
	n.link(n.at("elsewhere", "shots", "shot.png"), "project", "link.png")

	path := n.at("project", "shots", "shot.png")
	if got, err := readOf(t, path, rootOwns); err != nil || got != "png" {
		t.Fatalf("read %q, %v", got, err)
	}
	if _, err := OpenNoLinks(n.at("project", "link.png")); !errors.Is(err, ErrLinkOnPath) {
		t.Errorf("a link at the end: %v", err)
	}
	// The VM swaps the folder for a link after devvm resolved the path.
	n.must(os.Rename(n.at("project", "shots"), n.at("project", "shots.was")))
	n.link(n.at("elsewhere", "shots"), "project", "shots")
	if _, err := OpenNoLinks(path); !errors.Is(err, ErrLinkOnPath) {
		t.Errorf("a link on the way: %v", err)
	}
	if _, err := OpenNoLinks("project/shots/shot.png"); err == nil || errors.Is(err, ErrLinkOnPath) {
		t.Errorf("a relative path: %v", err)
	}
	if _, err := OpenNoLinks(n.at("project") + "/"); err == nil {
		t.Error("an unclean path: opened")
	}
	if _, err := OpenNoLinks("/"); err == nil {
		t.Error("/: opened")
	}
	if _, err := OpenNoLinks(n.at("project", "missing.png")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
	f, err := OpenNoLinks(n.at("project"))
	if err == nil {
		fi, _ := f.Stat()
		_ = f.Close()
		if fi == nil || !fi.IsDir() {
			t.Error("a folder: not a folder")
		}
	}
}

// A link on the way that the follow check allows, as root's are, is
// followed: an absolute target starts over from /, a relative one goes
// on from the link's folder, .. included, a chain of them is followed
// link by link, and the file is read as named at the end. In the same
// tree, a link the check doesn't allow is refused, as is a followed
// link's target that's gone.
func TestOpenNoLinksFollowsSystemLinks(t *testing.T) {
	n := newNoLinksTree(t)
	n.file("real", "private", "tmp", "shots", "shot.png")
	n.link(n.at("private", "tmp"), "tmp")                       // absolute, as macOS's /tmp -> /private/tmp
	n.link("../private/tmp", "var", "tmp")                      // relative, through ..
	n.link("tmp/shots", "chain")                                // a link to a folder under a link
	n.link(n.at("private", "gone"), "dead")                     // a target that's gone
	n.link(n.at("private", "tmp"), "project", "docs")           // the user's own link, planted by a VM say
	n.link(n.at("private", "tmp", "shots", "shot.png"), "last") // a link in the file's place

	system := n.following(n.at("tmp"), n.at("var", "tmp"), n.at("chain"), n.at("dead"), n.at("last"))
	for _, p := range []string{
		n.at("tmp", "shots", "shot.png"),
		n.at("var", "tmp", "shots", "shot.png"),
		n.at("chain", "shot.png"),
	} {
		if got, err := readOf(t, p, system); err != nil || got != "real" {
			t.Errorf("%s: read %q, %v", p, got, err)
		}
	}
	if _, err := readOf(t, n.at("project", "docs", "shot.png"), system); !errors.Is(err, ErrLinkOnPath) {
		t.Errorf("the user's link on the way: %v", err)
	}
	if _, err := readOf(t, n.at("last"), system); !errors.Is(err, ErrLinkOnPath) {
		t.Errorf("a link in the file's place, even one the check allows: %v", err)
	}
	if _, err := readOf(t, n.at("dead", "shot.png"), system); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a followed link whose target is gone: %v", err)
	}
	// Without the check, the same system links are refused.
	if _, err := OpenNoLinks(n.at("tmp", "shots", "shot.png")); !errors.Is(err, ErrLinkOnPath) {
		t.Errorf("a link on the way that isn't root's: %v", err)
	}
}

// Two links pointing at each other end with ELOOP after the hop cap,
// instead of looping.
func TestOpenNoLinksHopCap(t *testing.T) {
	n := newNoLinksTree(t)
	n.link("b", "loop", "a")
	n.link("a", "loop", "b")
	follow := func(*unix.Stat_t) bool { return true }
	_, err := readOf(t, n.at("loop", "a", "shot.png"), follow)
	if !errors.Is(err, unix.ELOOP) {
		t.Errorf("a loop of links: %v", err)
	}
}
