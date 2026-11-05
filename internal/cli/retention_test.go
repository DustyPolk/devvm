package cli

import (
	"os"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

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
	if err := os.Chtimes(d.RetentionLock(), old, old); err != nil {
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
