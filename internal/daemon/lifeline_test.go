package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// gone returns the pid of a process that has ended.
func gone(t *testing.T) int {
	t.Helper()
	h, err := Spawn(Cmd{Path: "true"})
	if err != nil {
		t.Fatal(err)
	}
	<-h.Done()
	return h.PID()
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A lifeline holds while it holds what it held and its process (the
// first line) runs: not once a new start rewrites it, it's removed, or
// its process ends.
func TestLifelineHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.pid")
	if got, err := ReadLifeline(path); got != "" || err != nil {
		t.Errorf("none: %q, %v", got, err)
	}
	me := strconv.Itoa(os.Getpid())
	write(t, path, me+"\n123\n")
	l, ok := FollowLifeline(path, 0, time.Millisecond)
	if !ok || l.Life != me+"\n123" || !l.Holds() {
		t.Fatalf("FollowLifeline = %+v, %v", l, ok)
	}
	write(t, path, me+"\n456\n")
	if l.Holds() {
		t.Error("holds after a new start")
	}
	write(t, path, me+"\n123\n")
	if !l.Holds() {
		t.Error("doesn't hold what it held")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if l.Holds() {
		t.Error("holds once removed")
	}
	dead := strconv.Itoa(gone(t))
	write(t, path, dead+"\n")
	if (Lifeline{Path: path, Life: dead}).Holds() {
		t.Error("holds with its process gone")
	}
	for _, life := range []string{"", "x", "0", "-1", dead} {
		if LifeRuns(life) {
			t.Errorf("LifeRuns(%q)", life)
		}
	}
}

// Following a lifeline waits, a bounded time, for it to name a process
// that runs, as a proxy started before its VM does: past what the file
// held when it first looked (an earlier start's), but not past a new
// start's whose process is already gone (a VM that stopped between two
// looks). With no wait it looks once.
func TestFollowLifeline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ha.pid")
	if _, ok := FollowLifeline(path, 0, time.Millisecond); ok {
		t.Error("none: followed")
	}
	start := time.Now()
	if _, ok := FollowLifeline(path, 100*time.Millisecond, 10*time.Millisecond); ok || time.Since(start) < 80*time.Millisecond {
		t.Errorf("none: followed %v, after %v", ok, time.Since(start))
	}
	// An earlier start's, then this one's.
	write(t, path, strconv.Itoa(gone(t))+"\n")
	got := make(chan Lifeline, 1)
	go func() {
		l, _ := FollowLifeline(path, 10*time.Second, 10*time.Millisecond)
		got <- l
	}()
	time.Sleep(50 * time.Millisecond)
	select {
	case l := <-got:
		t.Fatalf("followed an earlier start's: %+v", l)
	default:
	}
	h, err := Spawn(Cmd{Path: "sleep", Args: []string{"30"}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop(time.Second)
	write(t, path, strconv.Itoa(h.PID())+"\n")
	select {
	case l := <-got:
		if l.Life != strconv.Itoa(h.PID()) || !l.Holds() {
			t.Errorf("followed %+v", l)
		}
		// It holds until its process ends.
		done := make(chan struct{})
		go func() { l.Wait(10 * time.Millisecond); close(done) }()
		h.Stop(time.Second)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Wait: still waiting with its process gone")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never followed")
	}
	// A new start's, already gone, ends the wait at once.
	write(t, path, strconv.Itoa(gone(t))+"\n")
	ended := make(chan bool, 1)
	go func() {
		_, ok := FollowLifeline(path, 10*time.Second, 10*time.Millisecond)
		ended <- ok
	}()
	time.Sleep(50 * time.Millisecond)
	write(t, path, strconv.Itoa(gone(t))+"\n")
	select {
	case ok := <-ended:
		if ok {
			t.Error("followed a lifeline whose process is gone")
		}
	case <-time.After(5 * time.Second):
		t.Error("still waiting past a new start's lifeline whose process is gone")
	}
}
