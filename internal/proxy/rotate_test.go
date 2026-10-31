package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// limitLogs sets logLimit for one test.
func limitLogs(t *testing.T, n int64) {
	t.Helper()
	old := logLimit
	logLimit = n
	t.Cleanup(func() { logLimit = old })
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // the test's own file
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// The event log: past the limit it's renamed to <log>.1, replacing the
// one there, and a new one starts; no line spans two files, and every
// file is the user's alone, whatever mode the log had.
func TestRotateRenames(t *testing.T) {
	limitLogs(t, 30)
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	if err := os.WriteFile(path, []byte("old 0123456789\n"), 0o644); err != nil { //nolint:gosec // a log left open to others
		t.Fatal(err)
	}
	r, err := openRotating(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if m := modeOf(t, path); m != 0o600 {
		t.Errorf("opened: mode %v", m)
	}
	write := func(s string) {
		t.Helper()
		if n, err := r.Write([]byte(s)); err != nil || n != len(s) {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	write("one 0123456789\n") // 30 bytes: at the limit, not past it
	if got := readLog(t, path); got != "old 0123456789\none 0123456789\n" || readLog(t, path+".1") != "" {
		t.Fatalf("at the limit: %q", got)
	}
	write("two\n")
	if a, b := readLog(t, path+".1"), readLog(t, path); a != "old 0123456789\none 0123456789\n" || b != "two\n" {
		t.Errorf("rotated: .1 %q, log %q", a, b)
	}
	write("three 0123456789 01234\n")
	write("four\n")
	if a, b := readLog(t, path+".1"), readLog(t, path); a != "two\nthree 0123456789 01234\n" || b != "four\n" {
		t.Errorf("rotated again, the oldest dropped: .1 %q, log %q", a, b)
	}
	for _, p := range []string{path, path + ".1"} {
		if m := modeOf(t, p); m != 0o600 {
			t.Errorf("%s: mode %v", filepath.Base(p), m)
		}
	}
	// A line longer than the limit goes whole into a new log.
	long := strings.Repeat("x", 40) + "\n"
	write(long)
	if a, b := readLog(t, path+".1"), readLog(t, path); a != "four\n" || b != long {
		t.Errorf("a long line: .1 %q, log %q", a, b)
	}
	// A log deleted while it's written starts again at the limit.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	write("five\n")
	if got := readLog(t, path); got != "five\n" {
		t.Errorf("deleted: log %q", got)
	}
}

// Writes from many connections at once during a rotation: each line
// whole, once, in one of the two files.
func TestRotateConcurrent(t *testing.T) {
	const writers, each = 16, 50
	line := func(w, i int) string { return fmt.Sprintf(`{"writer":%02d,"line":%03d,"pad":"0123456789"}`+"\n", w, i) }
	total := int64(writers * each * len(line(0, 0)))
	// One rotation: the first file takes most of the lines.
	limitLogs(t, total*3/4)
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	r, err := openRotating(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				if _, err := r.Write([]byte(line(w, i))); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	old, cur := readLog(t, path+".1"), readLog(t, path)
	if old == "" || cur == "" {
		t.Fatalf("no rotation: .1 %d bytes, log %d bytes", len(old), len(cur))
	}
	seen := map[string]int{}
	for _, l := range strings.SplitAfter(old+cur, "\n") {
		if l != "" {
			seen[l]++
		}
	}
	for w := range writers {
		for i := range each {
			if n := seen[line(w, i)]; n != 1 {
				t.Errorf("%q: %d times", line(w, i), n)
			}
			delete(seen, line(w, i))
		}
	}
	if len(seen) > 0 {
		t.Errorf("lines split or not written: %v", seen)
	}
}

// The proxy's own log is also its stderr, which devvm opened and which
// its last error goes to: it's copied to <log>.1 and emptied, so writes
// that bypass the rotation still reach the current log.
func TestRotateCopies(t *testing.T) {
	limitLogs(t, 20)
	path := filepath.Join(t.TempDir(), "vm.proxy.log")
	if err := os.WriteFile(path, []byte("started 0123\n"), 0o644); err != nil { //nolint:gosec // a log left open to others
		t.Fatal(err)
	}
	stderr, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()
	r, err := copyRotating(path, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("loaded\n")); err != nil { // 20 bytes: at the limit
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("failed upstream\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := stderr.WriteString("devvm: the last error\n"); err != nil {
		t.Fatal(err)
	}
	if a, b := readLog(t, path+".1"), readLog(t, path); a != "started 0123\nloaded\n" || b != "failed upstream\ndevvm: the last error\n" {
		t.Errorf(".1 %q, log %q", a, b)
	}
	for _, p := range []string{path, path + ".1"} {
		if m := modeOf(t, p); m != 0o600 {
			t.Errorf("%s: mode %v", filepath.Base(p), m)
		}
	}
}
