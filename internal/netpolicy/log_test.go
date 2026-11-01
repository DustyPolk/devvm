package netpolicy

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatEvent(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{`{"time":"T","event":"tunnel","host":"ok.test","port":443,"rule":"ok.test"}`, "T  allowed  ok.test:443  rule ok.test"},
		{`{"time":"T","event":"intercept","host":"a.test","port":443,"rule":"a.test"}`, "T  allowed  a.test:443  rule a.test, decrypted"},
		{`{"time":"T","event":"tunnel","host":"a.test","port":443,"rule":"a.test","upstream":"proxy.corp:8080"}`, "T  allowed  a.test:443  rule a.test, through proxy.corp:8080"},
		{`{"time":"T","event":"forward","host":"a.test","port":443,"method":"GET","path":"/x","secrets":["a","b"]}`, "T  allowed  a.test:443  GET /x, token: a,b"},
		{`{"time":"T","event":"blocked","host":"b.test","port":22,"reason":"no"}`, "T  BLOCKED  b.test:22  no"},
		{`{"time":"T","event":"blocked","host":"::1","port":80,"reason":"no"}`, "T  BLOCKED  [::1]:80  no"},
		{`{"time":"T\u001b[2J","event":"forward","host":"a.test","port":443,"method":"GET","path":"/\n\u009bx\t"}`, "T[2J  allowed  a.test:443  GET /x"},
		{`{"time":"T","event":"nix-cache","host":"cache.nixos.org","port":443,"method":"GET","path":"/nix/x.narinfo","reason":"hit"}`, "T  cache    cache.nixos.org:443  GET /nix/x.narinfo: hit"},
		{`{"time":"T","event":"link","host":"db","port":5432,"target":"devvm-db-x"}`, "T  link     db:5432  to devvm-db-x"},
		{`{"time":"T","event":"link_refused","host":"db","port":5432,"target":"devvm-db-x","reason":"unpublished"}`, "T  REFUSED  db:5432  to devvm-db-x: unpublished"},
		{`{"time":"T","event":"link_target_stopped","host":"db","port":80,"target":"devvm-db-x"}`, "T  REFUSED  db:80  to devvm-db-x: it isn't running"},
		{`{"time":"T","event":"link_limit","host":"db","port":80,"target":"devvm-db-x","reason":"256"}`, "T  REFUSED  db:80  256 connections carried already"},
		{`not json`, ""},
		{`{"time":"T"}`, ""},
		{``, ""},
	} {
		got, ok := FormatEvent(c.line)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("FormatEvent(%q) = %q, %v; want %q", c.line, got, ok, c.want)
		}
	}
}

func event(n int) string {
	return `{"time":"T` + strconv.Itoa(n) + `","event":"blocked","host":"a.test","port":443,"reason":"r"}` + "\n"
}

func shown(n int) string { return "T" + strconv.Itoa(n) + "  BLOCKED  a.test:443  r\n" }

func TestWriteLog(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	// The last line is still being written, without its newline.
	if err := os.WriteFile(path, []byte(event(1)+event(2)+"junk\n"+strings.TrimSuffix(event(3), "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		n    int
		want string
	}{
		{50, shown(1) + shown(2) + shown(3)},
		{2, shown(2) + shown(3)},
		{0, ""},
	} {
		var b bytes.Buffer
		if err := (&Log{Path: path}).Write(t.Context(), &b, c.n, false); err != nil || b.String() != c.want {
			t.Errorf("n=%d: %q, %v; want %q", c.n, b.String(), err, c.want)
		}
	}
}

// syncBuffer is a bytes.Buffer safe to read while Log.Write writes it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestWriteLogFollow(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	ev3 := event(3)
	if err := os.WriteFile(path, []byte(event(1)+event(2)+ev3[:10]), 0o600); err != nil {
		t.Fatal(err)
	}
	appendTo := func(s string) {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the test's own file
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	var out syncBuffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	l := &Log{Path: path, Tick: 5 * time.Millisecond}
	go func() { done <- l.Write(ctx, &out, 1, true) }()
	wait := func(want string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); out.String() != want; {
			if time.Now().After(deadline) {
				t.Fatalf("followed: %q; want %q", out.String(), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	// The last whole one, then the one being written once it ends, then
	// each new one.
	wait(shown(2))
	appendTo(ev3[10:])
	wait(shown(2) + shown(3))
	appendTo(event(4))
	wait(shown(2) + shown(3) + shown(4))
	// Emptied and written again: read from its start.
	if err := os.WriteFile(path, []byte(event(5)), 0o600); err != nil {
		t.Fatal(err)
	}
	wait(shown(2) + shown(3) + shown(4) + shown(5))
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A log the proxy rotated: <log>.1's events come first, so the history
// is continuous; the blocks count both files'.
func TestWriteLogRotated(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	if err := os.WriteFile(path+".1", []byte(event(1)+event(2)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(event(3)), 0o600); err != nil {
		t.Fatal(err)
	}
	l := &Log{Path: path}
	for n, want := range map[int]string{50: shown(1) + shown(2) + shown(3), 2: shown(2) + shown(3)} {
		var b bytes.Buffer
		if err := l.Write(t.Context(), &b, n, false); err != nil || b.String() != want {
			t.Errorf("n=%d: %q, %v; want %q", n, b.String(), err, want)
		}
	}
	var b bytes.Buffer
	if err := l.WriteBlocked(&b); err != nil || !strings.Contains(b.String(), "T3    3      a.test:443") {
		t.Errorf("WriteBlocked: %q, %v", b.String(), err)
	}
	// Just rotated, before the next event: the old file's.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	b.Reset()
	if err := l.Write(t.Context(), &b, 50, false); err != nil || b.String() != shown(3) {
		t.Errorf("just rotated: %q, %v", b.String(), err)
	}
}

// devvm network log --follow goes on across a rotation: the renamed log is read to its
// end, then the new one from its start.
func TestWriteLogFollowRotated(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	if err := os.WriteFile(path, []byte(event(1)), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out syncBuffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	l := &Log{Path: path, Tick: 5 * time.Millisecond}
	go func() { done <- l.Write(ctx, &out, 50, true) }()
	wait := func(want string) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); out.String() != want; {
			if time.Now().After(deadline) {
				t.Fatalf("followed: %q; want %q", out.String(), want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(shown(1))
	// The proxy rotates as the proxy does: the last line goes to the
	// renamed file, then the new one starts.
	for i, step := range []func() error{
		func() error { _, err := f.WriteString(event(2)); return err },
		func() error { return os.Rename(path, path+".1") },
		func() error { _, err := f.WriteString(event(3)); return err },
		func() error {
			nf, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // the test's own file
			if err != nil {
				return err
			}
			_ = f.Close()
			f = nf
			_, err = f.WriteString(event(4))
			return err
		},
	} {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	wait(shown(1) + shown(2) + shown(3) + shown(4))
	if _, err := f.WriteString(event(5)); err != nil {
		t.Fatal(err)
	}
	wait(shown(1) + shown(2) + shown(3) + shown(4) + shown(5))
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A start looks for the proxy's line after what the log had: in the log,
// or, when the proxy rotated it as it started, in the rest of <log>.1 and
// the new log.
func TestLogged(t *testing.T) {
	t.Parallel()
	want := []byte("listening on 127.0.0.1:1 for vm")
	long := strings.Repeat("earlier\n", 10)
	for _, c := range []struct {
		old, log string
		before   int
		ok       bool
	}{
		{"", "before\nlistening on 127.0.0.1:1 for vm\n", 7, true},
		{"", "listening on 127.0.0.1:1 for vm\nafter\n", 32, false}, // an earlier start's
		{"before\nlistening on 127.0.0.1:1 for vm\n", "x\n", 7, true},
		{long, "listening on 127.0.0.1:1 for vm\n", len(long), true},
		{"listening on 127.0.0.1:1 for vm\n", "x\n", 32, false}, // before what the log had
		{"before\n", "x\n", 7, false},
	} {
		path := filepath.Join(t.TempDir(), "vm.proxy.log")
		if c.old != "" {
			if err := os.WriteFile(path+".1", []byte(c.old), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(path, []byte(c.log), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := logged(path, int64(c.before), want); got != c.ok {
			t.Errorf(".1 %q, log %q: %v, want %v", c.old, c.log, got, c.ok)
		}
	}
}

func TestWriteBlocked(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "vm.jsonl")
	lines := `{"time":"T2","event":"blocked","host":"a.test","port":443,"reason":"r"}
{"time":"T1","event":"tunnel","host":"ok.test","port":443,"rule":"ok.test"}
{"time":"T3","event":"blocked","host":"db.test","port":5432,"reason":"r"}
{"time":"T4","event":"blocked","host":"a.test","port":443,"reason":"r"}
{"time":"T1","event":"blocked","host":"fd00::1","port":22,"reason":"r"}
{"time":"T1","event":"blocked","host":"Bad Host","port":443,"reason":"r"}
{"time":"T5","event":"blocked","host":"api.github.com","port":443,"reason":"r","refused":true}`
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := (&Log{Path: path}).WriteBlocked(&b); err != nil {
		t.Fatal(err)
	}
	want := "LAST  COUNT  HOST                ALLOW WITH\n" +
		"T1    1      Bad Host:443        -\n" +
		"T1    1      [fd00::1]:22        devvm network allow [fd00::1]:22\n" +
		"T3    1      db.test:5432        devvm network allow db.test:5432\n" +
		"T4    2      a.test:443          devvm network allow a.test\n" +
		"T5    1      api.github.com:443  -\n"
	if b.String() != want {
		t.Errorf("WriteBlocked:\n%s\nwant:\n%s", b.String(), want)
	}
	blocks, err := (&Log{Path: path}).Blocks()
	if err != nil || len(blocks) != 5 {
		t.Fatalf("Blocks = %+v, %v", blocks, err)
	}
	if got, want := blocks[3], (Block{Last: "T4", Count: 2, Host: "a.test", Port: 443, Allow: "a.test"}); got != want {
		t.Errorf("Blocks[3] = %+v, want %+v", got, want)
	}
	if blocks[4].Allow != "" {
		t.Errorf("a refused block has a rule: %+v", blocks[4])
	}
	events, err := (&Log{Path: path}).Events(3)
	if err != nil || len(events) != 3 || events[0].Host != "fd00::1" || events[2].Host != "api.github.com" {
		t.Errorf("Events(3) = %+v, %v", events, err)
	}
}

// A link's events go into the VM's event log, the user's alone, beside
// the proxy's, with the time and the VM; a log past the proxy's limit is
// rotated first.
func TestAppendEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "devvm-app.jsonl")
	if err := os.WriteFile(path, []byte(event(1)), 0o644); err != nil { //nolint:gosec // a mode AppendEvent leaves
		t.Fatal(err)
	}
	if err := AppendEvent(path, "devvm-app", Event{Event: "link", Host: "db", Port: 5432, Target: "devvm-db"}); err != nil {
		t.Fatal(err)
	}
	events, err := (&Log{Path: path}).Events(10)
	if err != nil || len(events) != 2 || events[1].VM != "devvm-app" || events[1].Time == "" || events[1].Target != "devvm-db" {
		t.Fatalf("Events = %+v, %v", events, err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the log: %v, %v", fi, err)
	}
	if err := os.Truncate(path, eventLogMax); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(path, "devvm-app", Event{Event: "link_limit", Host: "db", Port: 5432, Reason: "256"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path + ".1"); err != nil || fi.Size() != eventLogMax {
		t.Errorf("the old log: %v, %v", fi, err)
	}
	if b, err := os.ReadFile(path); err != nil || strings.Count(string(b), "\n") != 1 || !strings.Contains(string(b), `"link_limit"`) { //nolint:gosec // the test's own file
		t.Errorf("the new log: %q, %v", b, err)
	}
	nf := filepath.Join(t.TempDir(), "new.jsonl")
	if err := AppendEvent(nf, "devvm-app", Event{Event: "link"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(nf); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("a new log: %v, %v", fi, err)
	}
}
