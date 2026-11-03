package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A record reads back as written, as JSON in the image's folder, and
// RemoveImage deletes the folder.
func TestImage(t *testing.T) {
	d := testDirs(t)
	built := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	want := Image{Name: "base", Chain: "base=0123456789ab", From: "alpine:3", Digest: "alpine@sha256:aa", Libc: "musl", Built: built}
	if err := WriteImage(d, "0123456789ab", want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadImage(d, "0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	data := read(t, filepath.Join(d.Images(), "0123456789ab", "record.json"))
	for _, line := range []string{`"name": "base"`, `"from": "alpine:3"`, `"built": "2026-11-02T10:00:00Z"`} {
		if !strings.Contains(data, "\n  "+line) {
			t.Errorf("record.json lacks the line %s:\n%s", line, data)
		}
	}
	if err := RemoveImage(d, "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.Image("0123456789ab")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after RemoveImage, its folder: %v", err)
	}
	if _, err := ReadImage(d, "0123456789ab"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after RemoveImage: got %v, want fs.ErrNotExist", err)
	}
}

// Fields a record doesn't know are ignored: a newer devvm's read.
func TestImageUnknownFields(t *testing.T) {
	d := testDirs(t)
	write(t, d.ImageRecord("0123456789ab"), `{"name": "base", "libc": "gnu", "made_by": "9.9.9", "new": {"x": 1}}`)
	got, err := ReadImage(d, "0123456789ab")
	if err != nil || got.Name != "base" || got.Libc != "gnu" {
		t.Errorf("got %+v, %v", got, err)
	}
}

// AnyImage counts only records: folders named like a key, with one.
func TestAnyImage(t *testing.T) {
	d := testDirs(t)
	if found, err := AnyImage(d); err != nil || found {
		t.Errorf("no images folder: %v, %v", found, err)
	}
	if err := SetFromDigest(d, "alpine:3", "sha256:aa"); err != nil {
		t.Fatal(err)
	}
	if err := BumpRebuild(d, "lima", "", "base"); err != nil {
		t.Fatal(err)
	}
	if _, err := ImageLock(d, "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	write(t, d.ImageRecord("0123456789zz"), `{"name": "base"}`)
	write(t, d.ImageRecord("0123456789a"), `{"name": "base"}`)
	write(t, d.Image("0123456789ab"), "a file, not a folder")
	if err := os.MkdirAll(d.Image("0123456789cd"), 0o700); err != nil { // a build that didn't finish
		t.Fatal(err)
	}
	if found, err := AnyImage(d); err != nil || found {
		t.Errorf("no records: %v, %v", found, err)
	}
	if err := WriteImage(d, "0123456789ef", Image{Name: "base"}); err != nil {
		t.Fatal(err)
	}
	if found, err := AnyImage(d); err != nil || !found {
		t.Errorf("a record: %v, %v", found, err)
	}
}

// Images reads every record that reads, by key.
func TestImages(t *testing.T) {
	d := testDirs(t)
	if got, err := Images(d); err != nil || len(got) != 0 {
		t.Errorf("no images folder: %v, %v", got, err)
	}
	if err := WriteImage(d, "0123456789ab", Image{Name: "base"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteImage(d, "0123456789cd", Image{Name: "foo"}); err != nil {
		t.Fatal(err)
	}
	write(t, d.ImageRecord("0123456789ef"), "not JSON")
	got, err := Images(d)
	if err != nil || len(got) != 2 || got["0123456789ab"].Name != "base" || got["0123456789cd"].Name != "foo" {
		t.Errorf("got %+v, %v", got, err)
	}
}

// ImageLock is a key's lock in the locks folder, made; a key that isn't a
// file name is refused, and nothing is made for it.
func TestImageLock(t *testing.T) {
	d := testDirs(t)
	path, err := ImageLock(d, "0123456789ab")
	if err != nil || path != filepath.Join(d.Images(), "locks", "0123456789ab") {
		t.Errorf("ImageLock = %q, %v", path, err)
	}
	if fi, err := os.Stat(filepath.Dir(path)); err != nil || !fi.IsDir() {
		t.Errorf("its folder: %v", err)
	}
	for _, key := range []string{"", "../k", ".k", `a\b`} {
		if _, err := ImageLock(d, key); err == nil {
			t.Errorf("ImageLock(%q): want an error", key)
		}
	}
}

func TestImageBad(t *testing.T) {
	d := testDirs(t)
	for name, data := range map[string]string{
		"no name":    `{"chain": "base=abc"}`,
		"empty file": "",
		"not JSON":   "name=base\n",
		"a bad type": `{"name": 3}`,
	} {
		t.Run(name, func(t *testing.T) {
			write(t, d.ImageRecord("k"), data)
			_, err := ReadImage(d, "k")
			wantCorrupt(t, err, d.ImageRecord("k"), 0)
		})
	}
	if err := WriteImage(d, "k", Image{Chain: "x"}); err == nil {
		t.Error("WriteImage without a name: want an error")
	}
	if err := WriteImage(d, "../k", Image{Name: "base"}); err == nil {
		t.Error("WriteImage to ../k: want an error")
	}
}

// Each bump writes a new counter for its backend, base and image alone:
// another backend's, another --from's (the exact string: node:24 and
// node-24 are two) and another image's stay.
func TestBumpRebuild(t *testing.T) {
	d := testDirs(t)
	counter := func(backend, from, name string) string {
		t.Helper()
		rs, err := ReadRebuilds(d)
		if err != nil {
			t.Fatal(err)
		}
		return rs.Counter(backend, from, name)
	}
	bump := func(backend, from, name string) {
		t.Helper()
		if err := BumpRebuild(d, backend, from, name); err != nil {
			t.Fatal(err)
		}
	}
	if rs, err := ReadRebuilds(d); err != nil || rs != nil {
		t.Errorf("no file: %v, %v", rs, err)
	}
	bump("lima", "", "foo")
	first := counter("lima", "", "foo")
	if first == "" {
		t.Fatal("no counter after a bump")
	}
	bump("lima", "", "foo")
	if second := counter("lima", "", "foo"); second == first || second == "" {
		t.Errorf("counter %q, then %q", first, second)
	}
	lima := counter("lima", "", "foo")
	bump("fake", "", "foo")
	bump("lima", "node:24", "foo")
	bump("lima", "node-24", "foo")
	bump("lima", "", "base")
	if counter("lima", "", "foo") != lima {
		t.Error("another backend's, base's or image's bump changed lima's foo")
	}
	if a, b := counter("lima", "node:24", "foo"), counter("lima", "node-24", "foo"); a == "" || b == "" || a == b {
		t.Errorf("node:24's counter %q, node-24's %q: want two", a, b)
	}
	if c := counter("qemu", "", "foo"); c != "" {
		t.Errorf("qemu's foo, never bumped: %q", c)
	}
	rs, err := ReadRebuilds(d)
	if err != nil || len(rs) != 5 {
		t.Errorf("%d counters, %v; want 5", len(rs), err)
	}
	if !strings.Contains(read(t, d.Rebuilds()), `"from": "node:24"`) {
		t.Error("rebuilds.json doesn't hold the --from as given")
	}
	for _, bad := range [][2]string{{"", "foo"}, {"lima", ""}} {
		if err := BumpRebuild(d, bad[0], "", bad[1]); err == nil {
			t.Errorf("BumpRebuild(%q, %q): want an error", bad[0], bad[1])
		}
	}
	write(t, d.Rebuilds(), "foo=1\n")
	_, err = ReadRebuilds(d)
	wantCorrupt(t, err, d.Rebuilds(), 0)
}

func TestFromDigests(t *testing.T) {
	d := testDirs(t)
	get := func(ref string) string {
		t.Helper()
		v, err := FromDigest(d, ref)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if got := get("node:24"); got != "" {
		t.Errorf("no file: got %q", got)
	}
	steps := [][2]string{
		{"node:24", "node@sha256:aaaa"},
		{"node:24", "node@sha256:bbbb"},
		{"ghcr.io/o/r:t", "ghcr.io/o/r@sha256:cccc"},
	}
	for _, s := range steps {
		if err := SetFromDigest(d, s[0], s[1]); err != nil {
			t.Fatal(err)
		}
		if got := get(s[0]); got != s[1] {
			t.Errorf("%s: got %q, want %q", s[0], got, s[1])
		}
	}
	if got := get("node:24"); got != "node@sha256:bbbb" {
		t.Errorf("node:24 replaced by another reference: %q", got)
	}
	if !strings.HasPrefix(d.FromDigests(), d.Cache()+string(filepath.Separator)) {
		t.Errorf("%s isn't in the cache", d.FromDigests())
	}
	for _, bad := range [][2]string{{"a b", "x"}, {"a=b", "x"}, {"", "x"}, {"a", "x\ny"}} {
		if err := SetFromDigest(d, bad[0], bad[1]); err == nil {
			t.Errorf("SetFromDigest(%q, %q): want an error", bad[0], bad[1])
		}
	}
	write(t, d.FromDigests(), "node:24=sha256:aaaa\n")
	_, err := FromDigest(d, "node:24")
	wantCorrupt(t, err, d.FromDigests(), 0)
}

// A tool image's record reads back as written; one without its hash, or
// not JSON, is corrupt.
func TestToolImage(t *testing.T) {
	d := testDirs(t)
	const file = "go-1.2.0-0123456789ab.erofs"
	built := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	want := ToolImage{SHA256: "aa", Libc: "gnu", Format: "plain-0123456789ab", Backend: "qemu", Built: built}
	if err := WriteToolImage(d, file, want); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadToolImage(d, file); err != nil || got != want {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
	if data := read(t, d.ToolRecord(file)); strings.Contains(data, "idle_since") {
		t.Errorf("a record that isn't idle has idle_since:\n%s", data)
	}
	want.IdleSince = built.Add(time.Hour)
	if err := WriteToolImage(d, file, want); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadToolImage(d, file); err != nil || got != want {
		t.Errorf("idle: got %+v, %v; want %+v", got, err, want)
	}
	if _, err := ReadToolImage(d, "go-1.1.0-0123456789ab.erofs"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("no record: %v", err)
	}
	for _, data := range []string{`{"libc": "gnu"}`, "sha256=aa"} {
		write(t, d.ToolRecord(file), data)
		_, err := ReadToolImage(d, file)
		wantCorrupt(t, err, d.ToolRecord(file), 0)
	}
	if err := WriteToolImage(d, file, ToolImage{Libc: "gnu"}); err == nil {
		t.Error("WriteToolImage without a hash: want an error")
	}
	for _, bad := range []string{"../x.erofs", ".x", ""} {
		if err := WriteToolImage(d, bad, want); err == nil {
			t.Errorf("WriteToolImage(%q): want an error", bad)
		}
		if _, err := ReadToolImage(d, bad); err == nil {
			t.Errorf("ReadToolImage(%q): want an error", bad)
		}
	}
}
