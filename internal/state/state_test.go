package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/paths"
)

func testDirs(t *testing.T) paths.Dirs {
	t.Helper()
	tmp := t.TempDir()
	return paths.Dirs{
		Data:   filepath.Join(tmp, "data", "devvm"),
		Config: filepath.Join(tmp, "config", "devvm"),
		Lima:   filepath.Join(tmp, "lima"),
	}
}

// The data and config folders are made, or made the user's alone whatever
// mode they had, and what's in them keeps its own mode.
func TestMakePrivate(t *testing.T) {
	d := testDirs(t)
	if err := os.MkdirAll(filepath.Join(d.Config, "layers"), 0o755); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	if err := os.Chmod(d.Config, 0o777); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	if err := MakePrivate(d); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]fs.FileMode{d.Data: 0o700, d.Config: 0o700, filepath.Join(d.Config, "layers"): 0o755} {
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, want %v (%v)", dir, fi.Mode().Perm(), want, err)
		}
	}
	// One that can't be made fails.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MakePrivate(paths.Dirs{Data: filepath.Join(file, "devvm"), Config: d.Config}); err == nil {
		t.Error("a data folder under a file was made")
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a test file
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// wantCorrupt fails unless err is a CorruptError for path at line.
func wantCorrupt(t *testing.T, err error, path string, line int) {
	t.Helper()
	var c *CorruptError
	if !errors.As(err, &c) {
		t.Fatalf("got %v, want a CorruptError", err)
	}
	if c.Path != path || c.Line != line {
		t.Errorf("got %s line %d, want %s line %d", c.Path, c.Line, path, line)
	}
}

func TestParseKV(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		want     map[string]string
		badLine  int // a CorruptError on this line
		wantKeys []string
	}{
		{name: "empty", data: "", want: map[string]string{}},
		{name: "values may hold '='", data: "chain=base=abc node=def\n", want: map[string]string{"chain": "base=abc node=def"}},
		{name: "empty values", data: "from=\n", want: map[string]string{"from": ""}},
		{name: "blank lines and CRLF", data: "a=1\r\n\r\nb=2", want: map[string]string{"a": "1", "b": "2"}, wantKeys: []string{"a", "b"}},
		{name: "not key=value", data: "a=1\noops\n", badLine: 2},
		{name: "invalid key", data: "Image=x\n", badLine: 1},
		{name: "empty key", data: "=x\n", badLine: 1},
		{name: "a key twice", data: "a=1\nb=2\na=3\n", badLine: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, err := parseKV("f", []byte(tt.data), isWord)
			if tt.badLine != 0 {
				wantCorrupt(t, err, "f", tt.badLine)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(k.vals, tt.want) {
				t.Errorf("got %v, want %v", k.vals, tt.want)
			}
			if tt.wantKeys != nil && !reflect.DeepEqual(k.keys, tt.wantKeys) {
				t.Errorf("got keys %v, want %v", k.keys, tt.wantKeys)
			}
		})
	}
}

// WriteFileMode writes the file whole with the mode asked for, whatever the
// umask, and replaces one there with another mode; nothing else is left.
func TestWriteFileMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	path := filepath.Join(dir, "program")
	for _, tc := range []struct {
		data string
		mode fs.FileMode
	}{{"#!/bin/sh\n", 0o700}, {"#!/bin/sh\necho\n", 0o700}, {"data\n", 0o600}, {"cert\n", 0o644}, {"again\n", 0o755 | fs.ModeSetuid}} {
		if err := WriteFileMode(path, []byte(tc.data), tc.mode); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != tc.data {
			t.Errorf("got %q, want %q", got, tc.data)
		}
		if fi, err := os.Stat(path); err != nil || fi.Mode() != tc.mode.Perm() {
			t.Errorf("%v: mode %v, %v", tc.mode, fi.Mode(), err)
		}
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("the folder holds %v, %v", entries, err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("folder: %v, %v", fi.Mode(), err)
	}
	// A folder that can't be made fails.
	if err := WriteFileMode(filepath.Join(path, "x"), nil, 0o700); err == nil {
		t.Error("written under a file")
	}
	// One that can't be renamed over leaves no temporary file.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileMode(sub, []byte("x"), 0o700); err == nil {
		t.Error("a folder replaced")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Errorf("left %v", entries)
	}
}

// PrivateDir makes a folder, with those above it, 700; corrects one there
// with another mode; and refuses a file, or a link to a folder, leaving
// what the link points to as it was.
func TestPrivateDir(t *testing.T) {
	tmp := t.TempDir()
	made := filepath.Join(tmp, "a", "b")
	if err := PrivateDir(made); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(made); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Errorf("made: %v, %v", fi.Mode(), err)
	}
	open := filepath.Join(tmp, "open")
	if err := os.Mkdir(open, 0o755); err != nil { //nolint:gosec // the mode PrivateDir corrects
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(open, "f"), nil, 0o644); err != nil { //nolint:gosec // a mode left as it is
		t.Fatal(err)
	}
	if err := PrivateDir(open); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]fs.FileMode{open: 0o700, filepath.Join(open, "f"): 0o644} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, want %v (%v)", p, fi.Mode(), want, err)
		}
	}
	file := filepath.Join(tmp, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "target")
	if err := os.Mkdir(target, 0o755); err != nil { //nolint:gosec // a mode a link mustn't change
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{file, link, filepath.Join(file, "under")} {
		if err := PrivateDir(bad); err == nil {
			t.Errorf("%s: made private", bad)
		}
	}
	if fi, err := os.Stat(target); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("the link's folder: %v, %v", fi.Mode(), err)
	}
}

func TestWriteFileIsAtomicAndPrivate(t *testing.T) {
	d := testDirs(t)
	path := d.VMState("devvm-p-1")
	for _, data := range []string{"a=1\n", "a=2\n"} {
		if err := writeFile(path, []byte(data)); err != nil {
			t.Fatal(err)
		}
		if got := read(t, path); got != data {
			t.Errorf("got %q, want %q", got, data)
		}
	}
	entries, err := os.ReadDir(d.VMs())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("want only the state file, got %v", entries)
	}
	for path, want := range map[string]fs.FileMode{path: 0o600, d.VMs(): 0o700} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s: mode %o, want %o", path, got, want)
		}
	}
}

func TestVMSetGet(t *testing.T) {
	d := testDirs(t)
	name := "devvm-x-00000000000000"
	v, err := CreateVM(d, name, NewVM{Image: "node", Dir: "/p", Chain: "base=abc node=def", Backend: "lima"})
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get(KeyChain); got != "base=abc node=def" {
		t.Errorf("chain = %q", got)
	}
	for _, val := range []string{"on", "off"} {
		if err := v.Set("ssh_agent", val); err != nil {
			t.Fatal(err)
		}
	}
	again, err := ReadVM(d, name)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"ssh_agent": "off", KeyImage: "node", KeyDir: "/p", KeyBackend: "lima", KeyFrom: ""} {
		if got := again.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if n := strings.Count(read(t, d.VMState(name)), "ssh_agent="); n != 1 {
		t.Errorf("ssh_agent is in the file %d times, want 1", n)
	}
}

// Every VM CreateVM makes gets an ID of its own, a new one when it's
// made again under the same name.
func TestVMID(t *testing.T) {
	d := testDirs(t)
	hex32 := regexp.MustCompile(`^[0-9a-f]{32}$`)
	a, err := CreateVM(d, "vm", NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	first := a.Get(KeyID)
	again, err := CreateVM(d, "vm", NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadVM(d, "vm")
	if err != nil {
		t.Fatal(err)
	}
	if !hex32.MatchString(first) || back.Get(KeyID) != again.Get(KeyID) || again.Get(KeyID) == first {
		t.Errorf("IDs %q, then %q (read back as %q): want 32 hex digits, a new one each time", first, again.Get(KeyID), back.Get(KeyID))
	}
}

func TestVMSetKeepsOtherWriters(t *testing.T) {
	d := testDirs(t)
	a, err := CreateVM(d, "vm", NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReadVM(d, "vm")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Set("net", "on"); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("ports", "all"); err != nil {
		t.Fatal(err)
	}
	if a.Get("net") != "on" || a.Get("ports") != "all" {
		t.Errorf("got net=%q ports=%q", a.Get("net"), a.Get("ports"))
	}
}

func TestVMMissing(t *testing.T) {
	d := testDirs(t)
	v, err := ReadVM(d, "devvm-none-00000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if v.Exists() || v.Get(KeyImage) != "" {
		t.Errorf("missing VM: exists=%v image=%q", v.Exists(), v.Get(KeyImage))
	}
	if err := v.Set("net", "off"); err != nil {
		t.Fatal(err)
	}
	if !v.Exists() {
		t.Error("after Set: want the state file")
	}
	if err := v.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.VMState(v.Name())); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after Remove: %v", err)
	}
	if err := v.Remove(); err != nil {
		t.Errorf("removing twice: %v", err)
	}
}

func TestVMNames(t *testing.T) {
	d := testDirs(t)
	if names, err := VMNames(d); err != nil || names != nil {
		t.Fatalf("no state folder: %v, %v", names, err)
	}
	write(t, d.VMState("devvm-b-00000000000000"), "net=off\n")
	write(t, d.VMState("devvm-a-00000000000000"), "net=off\n")
	write(t, d.VMState(".devvm-c-00000000000000.tmp-1"), "")
	write(t, d.VMState("devvm-a-00000000000000.policy"), "example.com\n")
	if err := os.MkdirAll(filepath.Join(d.VMs(), "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	names, err := VMNames(d)
	want := []string{"devvm-a-00000000000000", "devvm-b-00000000000000"}
	if err != nil || !reflect.DeepEqual(names, want) {
		t.Errorf("VMNames = %v, %v; want %v", names, err, want)
	}
}

func TestVMCorrupt(t *testing.T) {
	d := testDirs(t)
	write(t, d.VMState("vm"), "image=base\ngarbage\n")
	_, err := ReadVM(d, "vm")
	wantCorrupt(t, err, d.VMState("vm"), 2)
}

func TestVMRefuses(t *testing.T) {
	d := testDirs(t)
	for _, name := range []string{"", ".", "..", ".hidden", "a/b", "../x"} {
		if _, err := ReadVM(d, name); err == nil {
			t.Errorf("ReadVM(%q): want an error", name)
		}
		if _, err := CreateVM(d, name, NewVM{}); err == nil {
			t.Errorf("CreateVM(%q): want an error", name)
		}
	}
	v, err := CreateVM(d, "vm", NewVM{Image: "base"})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{{"Net", "on"}, {"a=b", "c"}, {"", "x"}, {"net", "on\nimage=evil"}} {
		if err := v.Set(kv[0], kv[1]); err == nil {
			t.Errorf("Set(%q, %q): want an error", kv[0], kv[1])
		}
	}
	if _, err := CreateVM(d, "vm2", NewVM{Dir: "/a\nb"}); err == nil {
		t.Error("CreateVM with a line break: want an error")
	}
}

func TestImage(t *testing.T) {
	d := testDirs(t)
	want := Image{Name: "base", Chain: "base=0123456789ab", From: "alpine:3", Digest: "alpine@sha256:aa", Libc: "musl"}
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
	if err := RemoveImage(d, "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadImage(d, "0123456789ab"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after RemoveImage: got %v, want fs.ErrNotExist", err)
	}
}

// AnyImage counts only records: files named like a key.
func TestAnyImage(t *testing.T) {
	d := testDirs(t)
	if found, err := AnyImage(d); err != nil || found {
		t.Errorf("no images folder: %v, %v", found, err)
	}
	if err := SetFromDigest(d, "alpine:3", "sha256:aa"); err != nil {
		t.Fatal(err)
	}
	if err := BumpRebuild(d, "base", ""); err != nil {
		t.Fatal(err)
	}
	write(t, d.ImageState("0123456789zz"), "name=base\n")
	write(t, d.ImageState("0123456789a"), "name=base\n")
	if found, err := AnyImage(d); err != nil || found {
		t.Errorf("no records: %v, %v", found, err)
	}
	if err := WriteImage(d, "0123456789ab", Image{Name: "base"}); err != nil {
		t.Fatal(err)
	}
	if found, err := AnyImage(d); err != nil || !found {
		t.Errorf("a record: %v, %v", found, err)
	}
}

// ImageLock is a key's lock in its folder, made; a key that isn't a file
// name is refused, and nothing is made for it.
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
	tests := []struct {
		name, data string
		line       int
	}{
		{"no name", "chain=base=abc\n", 0},
		{"empty file", "", 0},
		{"unknown key", "name=base\nsize=3\n", 2},
		{"not key=value", "base\nbase=abc\n", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			write(t, d.ImageState("k"), tt.data)
			_, err := ReadImage(d, "k")
			wantCorrupt(t, err, d.ImageState("k"), tt.line)
		})
	}
	if err := WriteImage(d, "k", Image{Chain: "x"}); err == nil {
		t.Error("WriteImage without a name: want an error")
	}
	if err := WriteImage(d, "../k", Image{Name: "base"}); err == nil {
		t.Error("WriteImage to ../k: want an error")
	}
}

// Each bump writes a new counter, apart for each --from.
func TestBumpRebuild(t *testing.T) {
	d := testDirs(t)
	if err := BumpRebuild(d, "foo", ""); err != nil {
		t.Fatal(err)
	}
	first := read(t, d.RebuildFile("foo", ""))
	if err := BumpRebuild(d, "foo", ""); err != nil {
		t.Fatal(err)
	}
	if second := read(t, d.RebuildFile("foo", "")); second == first || second == "" {
		t.Errorf("counter %q, then %q", first, second)
	}
	if err := BumpRebuild(d, "foo", "node:24"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(d.RebuildFile("foo", "node:24")); err != nil {
		t.Error(err)
	}
	if err := BumpRebuild(d, "../x", ""); err == nil {
		t.Error("BumpRebuild(../x): want an error")
	}
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
	if n := strings.Count(read(t, d.FromDigests()), "node:24="); n != 1 {
		t.Errorf("node:24 recorded %d times, want 1", n)
	}
	if err := SetFromDigest(d, "a b", "x"); err == nil {
		t.Error("a reference with a space: want an error")
	}
	write(t, d.FromDigests(), "node:24 sha256:aaaa\n")
	_, err := FromDigest(d, "node:24")
	wantCorrupt(t, err, d.FromDigests(), 1)
}
