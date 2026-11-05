package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

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

// An orphan's leftover state is its whole folder: its state, logs and
// run files, the temporary files of its unfinished writes; a VM
// its backend has is none, and its logs are logs. A VM whose
// backend didn't answer is never an orphan.
func TestLeftoverFolders(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	const orphan, live, unasked = "devvm-a-0123456789abc1", "devvm-b-0123456789abc2", "devvm-c-0123456789abc3"
	for vm, be := range map[string]string{orphan: "fake", live: "fake", unasked: "other"} {
		v := d.VM(vm)
		for _, f := range []string{v.Logs() + "/network.jsonl", v.ProxyPID(), v.ForwarderLog(), filepath.Join(v.Run(), ".proxy.json.tmp-1")} {
			if err := state.WriteFile(f, []byte(strings.Repeat("x", 5000))); err != nil {
				t.Fatal(err)
			}
		}
		st, err := state.New(d, vm)
		if err != nil {
			t.Fatal(err)
		}
		st.Image, st.Dir, st.Backend = "base", "/p", be
		if err := st.Create(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
	}
	// A VM whose state can't be read is never an orphan either: its logs
	// are logs.
	broken := d.VM("devvm-d-0123456789abc4")
	for f, data := range map[string]string{broken.State(): "[]", broken.EventLog(): strings.Repeat("x", 5000)} {
		if err := state.WriteFile(f, []byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	fake := backendtest.New("fake", nil)
	fake.Instances[live] = backend.Instance{Name: live, Status: backend.StatusStopped}
	other := backendtest.New("other", nil)
	other.Errs = map[string]error{"List": errors.New("no")}
	reg, err := backend.NewRegistry(fake, other)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := vm.ReadInventory(t.Context(), d, reg, vm.AllBackends)
	if err != nil {
		t.Fatal(err)
	}
	e := spaceEnv{b: &build.Env{Dirs: d}, inv: inv, answered: map[string]bool{"fake": true}, now: time.Now()}
	row, err := e.leftovers(t.Context())
	if err != nil || row.Count != 1 || row.Size != du(d.VM(orphan).Dir) || row.Size == 0 {
		t.Errorf("leftovers = %+v, %v; want %s's folder, %d", row, err, orphan, du(d.VM(orphan).Dir))
	}
	logs, err := e.vmLogs()
	// Every log of theirs: the network logs and the port forwarders'.
	want := du(d.VM(live).EventLog()) + du(d.VM(unasked).EventLog()) + du(d.VM(live).ForwarderLog()) + du(d.VM(unasked).ForwarderLog()) + du(broken.EventLog())
	if err != nil || logs.count != 5 || logs.size != want {
		t.Errorf("vmLogs = %+v, %v; want 5, %d", logs, err, want)
	}
}

func TestParseExportName(t *testing.T) {
	for _, c := range []struct {
		name, vm, stamp string
		ok              bool
	}{
		{"devvm-web-0123456789abcd-20260101-120000.tar.gz", "devvm-web-0123456789abcd", "20260101-120000", true},
		{"devvm-img-keep-0123456789abcd-20260101-120000.tar.gz", "devvm-img-keep-0123456789abcd", "20260101-120000", true},
		{"devvm-img-0123456789ab-20260101-120000.tar.gz", "", "", false},
		{"devvm-web-0123456789abcd-2026010-120000.tar.gz", "", "", false},
		{"devvm-web-0123456789abcd-20260101-120000.tar", "", "", false},
		{"devvm-web-0123456789abcd.tar.gz", "", "", false},
		{".devvm-export-1", "", "", false},
	} {
		if vm, stamp, ok := parseExportName(c.name); vm != c.vm || stamp != c.stamp || ok != c.ok {
			t.Errorf("parseExportName(%q) = %q, %q, %v", c.name, vm, stamp, ok)
		}
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
			{What: "images", Backend: "lima", Count: 2, Size: 2048, Sized: true, Kept: keptRemade, GC: 1024, Rest: "images rm"},
			{What: "VMs", Backend: "fake", Count: 1, Kept: keptOnlyCopy, Rest: "devvm rm"},
			{What: "history", Count: 1, Size: 1024, Sized: true, Kept: keptOnlyCopy, Rest: "gc --history", Retain: "30d", RetainFrees: 1024},
		},
		Total:  2048,
		GC:     1024,
		Caches: []cacheRow{{Backend: "lima", Name: "download cache", Count: 1, Size: 1 << 30, Sized: true, Path: "/c/lima"}},
	}
	got, colored := tables(t, func(tab *ui.Table) error { return writeSpace(tab, r) })
	want := "WHAT     COUNT  SIZE  KEPT       GC    FREED BY      RETAIN\n" +
		"images   2      2.0K  remade     1.0K  images rm     -\n" +
		"VMs      1      -     only copy  -     devvm rm      -\n" +
		"history  1      1.0K  only copy  -     gc --history  30d (1.0K)\n" +
		"total    -      2.0K  -          1.0K  -             -\n" +
		"\n" +
		"CACHE           COUNT  SIZE  PATH\n" +
		"download cache  1      1.0G  /c/lima\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	wantCodes(t, colored, "\033[1mWHAT\033[0m", "\033[1mtotal\033[0m", "\033[1m2.0K\033[0m", "\033[33monly copy\033[0m", "  remade  ", "\033[1mCACHE\033[0m")
}
