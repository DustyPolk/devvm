package gitprotect

import (
	"os"
	"path/filepath"
	"testing"
)

// containFixture is a project folder with a symlink out of it (out), one
// within it (in), a real subfolder (sub) with a file, and a sibling with
// the folder's name as its prefix.
func containFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t, "p")
	mkdirs(t, f.path("sub"), filepath.Join(f.root, "other"), f.dir+"-sibling")
	for link, target := range map[string]string{
		f.path("out"): filepath.Join(f.root, "other"),
		f.path("in"):  f.path("sub"),
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	write(t, f.path("sub/file"), "")
	return f
}

// into resolves the folder's symlinks but not the last element, and needs
// the path below the project folder, never the folder itself.
func TestIntoPins(t *testing.T) {
	f := containFixture(t)
	d := f.dir
	for _, tc := range []struct {
		path string
		want bool
	}{
		{d + "/sub/file", true},
		{d + "/sub/missing", true},
		{d + "/missing/deeper", true},
		{d + "/in/file", true},
		{d + "/out", true}, // the last element as named
		{d + "/out/x", false},
		{d + "/sub/../../p-sibling/x", false},
		{d + "/missing/../../p-sibling", false},
		{d + "/sub/", true},
		{d, false},
		{d + "/", true}, // its base is the folder's name: d/p
		{d + "-sibling/x", false},
		{f.root + "/other/../p/sub", true},
	} {
		if got := f.p.into(tc.path); got != tc.want {
			t.Errorf("into(%s) = %v", tc.path, got)
		}
	}
}

// realInDir takes the path as named: below the folder, not a symlink, and
// its parent folder exists with no symlink in it.
func TestRealInDirPins(t *testing.T) {
	f := containFixture(t)
	d := f.dir
	for _, tc := range []struct {
		path string
		want bool
	}{
		{d + "/sub/file", true},
		{d + "/sub/missing", true},
		{d + "/sub", true},
		{d + "/missing/x", false},
		{d + "/in", false},
		{d + "/in/file", false},
		{d + "/out/x", false},
		{d + "/sub/../sub/file", true}, // its parent is cleaned
		{d + "/sub/../in/file", false},
		{d, false},
		{d + "/", true}, // callers pass clean paths
		{d + "-sibling/x", false},
	} {
		if got := f.p.realInDir(tc.path); got != tc.want {
			t.Errorf("realInDir(%s) = %v", tc.path, got)
		}
	}
}

// hooksPathInside: an existing folder is resolved, anything else is
// compared cleaned, and the folder itself counts.
func TestHooksPathInsidePins(t *testing.T) {
	f := containFixture(t)
	d := f.dir
	for _, tc := range []struct {
		path string
		want bool
	}{
		{d, true},
		{d + "/", true},
		{d + "/in", true},
		{d + "/out", false},
		{d + "/out/missing", true}, // missing: compared by name
		{d + "/sub/file", true},    // a file isn't resolved
		{d + "-sibling", false},
		{d + "/sub/../../p-sibling", false},
		{"/", false},
	} {
		if got := f.p.hooksPathInside(tc.path); got != tc.want {
			t.Errorf("hooksPathInside(%s) = %v", tc.path, got)
		}
	}
}
