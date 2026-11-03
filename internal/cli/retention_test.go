package cli

import (
	"os"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
)

// history.age deletes those that old; history.count then keeps the newest;
// off deletes none.
func TestExportsToRetire(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	es := []savedExport{{VM: "a", At: now.Add(-40 * day)}, {VM: "b", At: now.Add(-20 * day)}, {VM: "c", At: now.Add(-10 * day)}, {VM: "d", At: now}}
	vms := func(es []savedExport) string {
		s := ""
		for _, e := range es {
			s += e.VM
		}
		return s
	}
	for _, tt := range []struct {
		r    settings.Retention
		want string
	}{
		{settings.Retention{}, ""},
		{settings.Retention{ExportsAge: 30 * day}, "a"},
		{settings.Retention{ExportsCount: 2}, "ab"},
		{settings.Retention{ExportsAge: 30 * day, ExportsCount: 2}, "ab"},
		{settings.Retention{ExportsAge: 15 * day, ExportsCount: 1}, "abc"},
		{settings.Retention{ExportsCount: 9}, ""},
	} {
		if got := vms(exportsToRetire(es, tt.r, now)); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt.r, got, tt.want)
		}
	}
}

// Retention runs at most once a day, again at once after the clock went
// back, and never in two devvms at once, however long one takes.
func TestRetainDueAndLock(t *testing.T) {
	d := paths.Dirs{Data: t.TempDir()}
	now := time.Now()
	if !retainDue(d, now) {
		t.Error("never ran: want due")
	}
	touchRetention(d)
	if retainDue(d, now.Add(time.Hour)) {
		t.Error("an hour after: want not due")
	}
	if !retainDue(d, now.Add(25*time.Hour)) {
		t.Error("a day after: want due")
	}
	if !retainDue(d, now.Add(-time.Hour)) {
		t.Error("a clock that went back: want due")
	}
	unlock, ok := lockRetention(d)
	if !ok {
		t.Fatal("first lock refused")
	}
	if _, ok := lockRetention(d); ok {
		t.Error("second lock taken while the first is held")
	}
	// However long a devvm has held it, another doesn't take it over.
	old := now.Add(-time.Hour)
	if err := os.Chtimes(retentionLock(d), old, old); err != nil {
		t.Fatal(err)
	}
	if _, ok := lockRetention(d); ok {
		t.Error("an hour-old lock taken while its devvm holds it")
	}
	unlock()
	unlock, ok = lockRetention(d)
	if !ok {
		t.Error("lock refused once released")
	} else {
		unlock()
	}
}
