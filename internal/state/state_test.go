package state

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
