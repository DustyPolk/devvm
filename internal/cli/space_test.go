package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

func fakeRegistry(t *testing.T) *backend.Registry {
	t.Helper()
	lima := backendtest.New("lima", backend.Caps{})
	smol := backendtest.New("smolvm", backend.Caps{})
	smol.Format = "plain-0123456789ab"
	dock := backendtest.New("docker", backend.Caps{})
	dock.Format = "dir-0123456789ab"
	reg, err := backend.NewRegistry(lima, smol, dock)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// A tool image's backend is the one whose format it has, or one of the
// same kind with another hash (an older devvm's script).
func TestFormatBackend(t *testing.T) {
	reg := fakeRegistry(t)
	for format, want := range map[string]string{
		"":                   "lima",
		"plain-0123456789ab": "smolvm",
		"plain-ffffffffffff": "smolvm",
		"dir-ffffffffffff":   "docker",
		"other-0123":         "",
	} {
		if got := formatBackend(reg, format); got != want {
			t.Errorf("formatBackend(%q) = %q, want %q", format, got, want)
		}
	}
}

// A tool image's target is its record's, else a VM's that has it, else
// unknown.
func TestToolTarget(t *testing.T) {
	reg := fakeRegistry(t)
	vms := map[string]vmTarget{"go-1.20.0-0123456789ab.erofs": {backend: "smolvm", libc: "musl"}}
	tests := []struct {
		l             tools.Listed
		backend, libc string
	}{
		{tools.Listed{File: "node-1-0123456789ab.erofs", Targeted: true, Target: tools.Target{Libc: "gnu", Format: "dir-0123456789ab"}}, "docker", "gnu"},
		{tools.Listed{File: "go-1.20.0-0123456789ab.erofs"}, "smolvm", "musl"},
		{tools.Listed{File: "go-1.9.0-0123456789ab.erofs"}, "", ""},
	}
	for _, tt := range tests {
		be, libc := toolTarget(reg, tt.l, vms)
		if be != tt.backend || libc != tt.libc {
			t.Errorf("%s: got %q %q, want %q %q", tt.l.File, be, libc, tt.backend, tt.libc)
		}
	}
}

// Counting a folder: only what's picked, and a folder that isn't there is
// empty.
func TestEntriesIn(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.tar.gz", "b.tar.gz", "c.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"v1", ".tmp-1"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	c, err := filesIn(dir, func(n string) bool { return strings.HasSuffix(n, ".tar.gz") })
	if err != nil || c.count != 2 || c.size <= 0 {
		t.Errorf("filesIn = %+v, %v; want 2 files", c, err)
	}
	if c, err := dirsIn(dir); err != nil || c.count != 1 {
		t.Errorf("dirsIn = %+v, %v; want 1 folder", c, err)
	}
	if c, err := dirsIn(filepath.Join(dir, "gone")); err != nil || c.count != 0 {
		t.Errorf("dirsIn of a missing folder = %+v, %v", c, err)
	}
}

// Leftovers are a VM's state, run files and backend records, never
// another VM's whose name starts the same.
func TestLeftoverFiles(t *testing.T) {
	dir := t.TempDir()
	d := paths.Dirs{Data: dir}
	files := []string{
		d.VMState("devvm-a-1"), filepath.Join(d.Run(), "devvm-a-1.proxy.pid"), filepath.Join(d.LimaVMs(), "devvm-a-1.ports"),
		filepath.Join(d.Run(), "devvm-a-12.proxy.pid"),
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := leftoverFiles(d, "devvm-a-1")
	if len(got) != 3 || strings.Contains(strings.Join(got, " "), "devvm-a-12") {
		t.Errorf("leftoverFiles = %v", got)
	}
}

// Exports: only plain files named as devvm names them, oldest first; the
// age filter keeps those at least that old.
func TestExports(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	if err := os.MkdirAll(d.Exports(), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		"devvm-b-12345612345612-20260301-000000.tar.gz", "devvm-a-12345612345612-20260101-120000.tar.gz",
		".devvm-export-1", "notes.tar.gz", "devvm-c-1234-20260101-000000.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(d.Exports(), f), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	es, err := exports(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].VM != "devvm-a-12345612345612" || es[1].VM != "devvm-b-12345612345612" {
		t.Fatalf("exports = %+v", es)
	}
	// Now is counted from b's parsed date, so the zone the test runs in
	// doesn't matter: exactly 30d after b both are that old, a second
	// short only a.
	age := 30 * 24 * time.Hour
	boundary := es[1].At.Add(age)
	if got := olderThan(es, age, boundary); len(got) != 2 {
		t.Errorf("olderThan 30d, at b's 30d = %+v", got)
	}
	if got := olderThan(es, age, boundary.Add(-time.Second)); len(got) != 1 || got[0].VM != "devvm-a-12345612345612" {
		t.Errorf("olderThan 30d, a second short = %+v", got)
	}
	if got := olderThan(es, 0, boundary); len(got) != 2 {
		t.Errorf("olderThan 0 = %+v", got)
	}
}

// The table: rows, a total, and the caches apart; what a row lacks is -.
func TestWriteSpace(t *testing.T) {
	r := spaceReport{
		Rows: []spaceRow{
			{What: "images", Backend: "lima", Count: 2, Size: 2048, Sized: true, Kept: keptRemade, GC: 1024, Rest: "gc --backend lima"},
			{What: "VMs", Backend: "docker", Count: 1, Kept: keptOnlyCopy, Rest: "devvm rm"},
			{What: "history", Count: 1, Size: 1024, Sized: true, Kept: keptOnlyCopy, Rest: "gc --history", Retain: "30d", RetainFrees: 1024},
		},
		Total:  2048,
		GC:     1024,
		Caches: []cacheRow{{Backend: "lima", Name: "download cache", Count: 1, Size: 1 << 30, Sized: true, Path: "/c/lima"}},
	}
	got, colored := tables(t, func(tab *ui.Table) error { return writeSpace(tab, r) })
	want := "WHAT     BACKEND  COUNT  SIZE  KEPT       GC    FREED BY           RETAIN\n" +
		"images   lima     2      2.0K  remade     1.0K  gc --backend lima  -\n" +
		"VMs      docker   1      -     only copy  -     devvm rm           -\n" +
		"history  -        1      1.0K  only copy  -     gc --history       30d (1.0K)\n" +
		"total    -        -      2.0K  -          1.0K  -                  -\n" +
		"\n" +
		"BACKEND  CACHE           COUNT  SIZE  PATH\n" +
		"lima     download cache  1      1.0G  /c/lima\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	wantCodes(t, colored, "\033[1mWHAT\033[0m", "\033[1mtotal\033[0m", "\033[1m2.0K\033[0m", "\033[33monly copy\033[0m", "  remade  ", "\033[1mBACKEND\033[0m")
}
