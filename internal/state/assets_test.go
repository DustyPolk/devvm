package state

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"base.yaml":          {Data: []byte("cpus: 4\n")},
		"layers/base.sh":     {Data: []byte("#!/bin/bash\n")},
		"oci/build.sh":       {Data: []byte("#!/bin/bash\necho oci\n")},
		"oci/README.md":      {Data: []byte("# oci\n")},
		"guest/tools.sh":     {Data: []byte("")},
		"layers/base.policy": {Data: []byte("github.com\n")},
	}
}

func hashOf(t *testing.T, fsys fs.FS) string {
	t.Helper()
	h, err := AssetsHash(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// The hash names every file and its contents, each apart from the next.
func TestAssetsHash(t *testing.T) {
	base := hashOf(t, testAssets())
	if len(base) != 16 || base != hashOf(t, testAssets()) {
		t.Fatalf("hash %q, or not the same twice", base)
	}
	for _, tc := range []struct {
		name   string
		change func(fstest.MapFS)
	}{
		{"a file's contents", func(m fstest.MapFS) { m["base.yaml"] = &fstest.MapFile{Data: []byte("cpus: 8\n")} }},
		{"a file added", func(m fstest.MapFS) { m["features/fzf.sh"] = &fstest.MapFile{Data: []byte("")} }},
		{"an empty file removed", func(m fstest.MapFS) { delete(m, "guest/tools.sh") }},
		{"a file renamed", func(m fstest.MapFS) { m["oci/readme.md"] = m["oci/README.md"]; delete(m, "oci/README.md") }},
		{"bytes moved from a name to its contents", func(m fstest.MapFS) {
			m["base.yam"] = &fstest.MapFile{Data: []byte("lcpus: 4\n")}
			delete(m, "base.yaml")
		}},
	} {
		m := testAssets()
		tc.change(m)
		if got := hashOf(t, m); got == base {
			t.Errorf("%s: the hash didn't change", tc.name)
		}
	}
}

// Written out whole into a folder named by the hash, private to the user;
// a folder already there is used as is.
func TestWriteAssets(t *testing.T) {
	d := testDirs(t)
	m := testAssets()
	dir, err := WriteAssets(d, m)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(d.Assets(), hashOf(t, m)) {
		t.Errorf("written to %s", dir)
	}
	for name, f := range m {
		path := filepath.Join(dir, name)
		if got := read(t, path); got != string(f.Data) {
			t.Errorf("%s: %q, want %q", name, got, f.Data)
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode %v, %v", name, fi.Mode(), err)
		}
	}
	for _, sub := range []string{"", "layers", "oci"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s/: mode %v, %v", sub, fi.Mode(), err)
		}
	}
	// Used as is the next time.
	write(t, filepath.Join(dir, "base.yaml"), "kept\n")
	if again, err := WriteAssets(d, m); err != nil || again != dir || read(t, filepath.Join(dir, "base.yaml")) != "kept\n" {
		t.Errorf("again: %s, %v", again, err)
	}
	// Other files, another folder; nothing left half-written.
	m["base.yaml"] = &fstest.MapFile{Data: []byte("cpus: 8\n")}
	other, err := WriteAssets(d, m)
	if err != nil || other == dir {
		t.Errorf("other files: %s, %v", other, err)
	}
	entries, err := os.ReadDir(d.Assets())
	if err != nil || len(entries) != 2 {
		t.Errorf("the assets folder holds %v, %v", entries, err)
	}
}

// Runs writing the same files at once all get the one complete folder.
func TestWriteAssetsConcurrently(t *testing.T) {
	d := testDirs(t)
	m := testAssets()
	var wg sync.WaitGroup
	dirs := make([]string, 8)
	errs := make([]error, len(dirs))
	for i := range dirs {
		wg.Go(func() { dirs[i], errs[i] = WriteAssets(d, m) })
	}
	wg.Wait()
	for i := range dirs {
		if errs[i] != nil || dirs[i] != dirs[0] {
			t.Errorf("run %d: %s, %v", i, dirs[i], errs[i])
		}
	}
	if got := read(t, filepath.Join(dirs[0], "oci", "build.sh")); got != "#!/bin/bash\necho oci\n" {
		t.Errorf("build.sh: %q", got)
	}
	entries, err := os.ReadDir(d.Assets())
	if err != nil || len(entries) != 1 {
		t.Errorf("the assets folder holds %v, %v", entries, err)
	}
}

// Stale: every folder but the current one and those kept, and what a run
// left writing one an hour or more ago.
func TestStaleAssets(t *testing.T) {
	d := testDirs(t)
	if got, err := StaleAssets(d, "current", nil); err != nil || got != nil {
		t.Errorf("no folder: %v, %v", got, err)
	}
	for _, n := range []string{"current", "old", "kept", ".tmp-new", ".tmp-old"} {
		if err := os.MkdirAll(filepath.Join(d.Assets(), n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	then := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(d.Assets(), ".tmp-old"), then, then); err != nil {
		t.Fatal(err)
	}
	got, err := StaleAssets(d, "current", map[string]bool{"kept": true})
	want := []string{filepath.Join(d.Assets(), ".tmp-old"), filepath.Join(d.Assets(), "old")}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("StaleAssets = %q, %v; want %q", got, err, want)
	}
}
