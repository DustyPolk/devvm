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
		{`{"time":"T","event":"forward","host":"a.test","port":443,"method":"GET","path":"/x","secrets":["a","b"]}`, "T  allowed  a.test:443  GET /x, token: a,b"},
		{`{"time":"T","event":"blocked","host":"b.test","port":22,"reason":"no"}`, "T  BLOCKED  b.test:22  no"},
		{`{"time":"T","event":"blocked","host":"::1","port":80,"reason":"no"}`, "T  BLOCKED  [::1]:80  no"},
		{`{"time":"T\u001b[2J","event":"forward","host":"a.test","port":443,"method":"GET","path":"/\n\u009bx\t"}`, "T[2J  allowed  a.test:443  GET /x"},
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
		if err := WriteLog(context.Background(), &b, path, c.n, false); err != nil || b.String() != c.want {
			t.Errorf("n=%d: %q, %v; want %q", c.n, b.String(), err, c.want)
		}
	}
}

// syncBuffer is a bytes.Buffer safe to read while WriteLog writes it.
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
	old := LogTick
	LogTick = 5 * time.Millisecond
	t.Cleanup(func() { LogTick = old })
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- WriteLog(ctx, &out, path, 1, true) }()
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

func TestWriteBlocked(t *testing.T) {
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
	if err := WriteBlocked(&b, path); err != nil {
		t.Fatal(err)
	}
	want := "LAST  COUNT  HOST                ALLOW WITH\n" +
		"T1    1      Bad Host:443        -\n" +
		"T1    1      [fd00::1]:22        devvm policy allow [fd00::1]:22\n" +
		"T3    1      db.test:5432        devvm policy allow db.test:5432\n" +
		"T4    2      a.test:443          devvm policy allow a.test\n" +
		"T5    1      api.github.com:443  -\n"
	if b.String() != want {
		t.Errorf("WriteBlocked:\n%s\nwant:\n%s", b.String(), want)
	}
}
