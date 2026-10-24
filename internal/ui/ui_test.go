package ui

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

// sandbox points ui's streams at buffers, with input in, and its run log
// into a temporary data folder, in names mode. It undoes it all afterwards.
func sandbox(t *testing.T, in string) (out, errOut *bytes.Buffer) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	t.Setenv("DEVVM_VERBOSE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	oldOut, oldErr, oldIn := stdout, stderr, stdin
	oldInTTY, oldErrTTY, oldWidth := stdinTTY, stderrTTY, termWidth
	stdout, stderr, stdin = out, errOut, bufio.NewReader(strings.NewReader(in))
	stdinTTY = func() bool { return false }
	stderrTTY = func() bool { return false }
	termWidth = func() int { return 0 }
	resetRun()
	t.Cleanup(func() {
		resetRun()
		stdout, stderr, stdin = oldOut, oldErr, oldIn
		stdinTTY, stderrTTY, termWidth = oldInTTY, oldErrTTY, oldWidth
	})
	return out, errOut
}

// resetRun forgets the run: its status line, log, cleanups and interrupt.
func resetRun() {
	stopStatus()
	runLog.Lock()
	if runLog.f != nil {
		_ = runLog.f.Close()
	}
	runLog.f, runLog.path, runLog.noted = nil, "", false
	runLog.Unlock()
	cleanups.Lock()
	cleanups.funcs = nil
	cleanups.Unlock()
	interrupted.Store(false)
}

// exitCode runs f, returning the code it exits with through Die, or -1.
func exitCode(f func()) (code int) {
	defer func() {
		if p := recover(); p != nil {
			e, ok := p.(exitError)
			if !ok {
				panic(p)
			}
			code = e.code
		}
	}()
	f()
	return -1
}

func TestStreamsAndPrefixes(t *testing.T) {
	m := msg.MsgUnknown("x")
	for _, tc := range []struct {
		name          string
		print         func(msg.M)
		stdout, stder string
	}{
		{"say", Say, "msg_unknown name=x\n", ""},
		{"info", Info, "==> msg_unknown name=x\n", ""},
		{"note", Note, "", "msg_unknown name=x\n"},
		{"warn", Warn, "", "devvm: msg_unknown name=x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut := sandbox(t, "")
			tc.print(m)
			if out.String() != tc.stdout || errOut.String() != tc.stder {
				t.Errorf("stdout %q, stderr %q; want %q, %q", out, errOut, tc.stdout, tc.stder)
			}
		})
	}
}

func TestWarnAndDiePrefixEveryLine(t *testing.T) {
	if got, want := prefixLines("devvm: ", "one\ntwo"), "devvm: one\ndevvm: two\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	_, errOut := sandbox(t, "")
	t.Setenv("DEVVM_MESSAGES", "")
	m := msg.MsgUnknown("a")
	Warn(m)
	if got, want := errOut.String(), "devvm: "+m.Text()+"\n"; got != want {
		t.Errorf("warn: got %q, want %q", got, want)
	}
}

func TestDiePrintsAndExits1(t *testing.T) {
	out, errOut := sandbox(t, "")
	if code := exitCode(func() { Die(msg.MsgUnknown("VM")) }); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if got, want := errOut.String(), "devvm: msg_unknown name=VM\n"; got != want || out.Len() != 0 {
		t.Errorf("stderr %q, stdout %q; want %q and nothing", got, out, want)
	}
	// Interrupted, it dies quietly.
	errOut.Reset()
	interrupted.Store(true)
	if code := exitCode(func() { Die(msg.MsgUnknown("VM")) }); code != 1 || errOut.Len() != 0 {
		t.Errorf("interrupted: exit code %d, stderr %q; want 1 and nothing", code, errOut)
	}
}

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		in   string
		bare bool
		want bool
	}{
		{"\n", false, true},
		{"y\n", false, true},
		{"n\n", false, false},
		{"N\n", true, false},
		{"no", false, false}, // the last line needn't end in a newline
	} {
		sandbox(t, tc.in)
		var q []msg.M
		if !tc.bare {
			q = append(q, msg.MsgUnknown("x"))
		}
		if got := Confirm(q...); got != tc.want {
			t.Errorf("Confirm with %q = %v, want %v", tc.in, got, tc.want)
		}
	}
	sandbox(t, "")
	if code := exitCode(func() { Confirm(msg.MsgUnknown("x")) }); code != 1 {
		t.Errorf("no answer at all: exit code %d, want 1", code)
	}
}

func TestAsk(t *testing.T) {
	for in, want := range map[string]bool{"y\n": true, "Yes\n": true, "n\n": false, "\n": false, "x\n": false} {
		sandbox(t, in)
		if got := Ask(msg.MsgUnknown("me")); got != want {
			t.Errorf("Ask with %q = %v, want %v", in, got, want)
		}
	}
	sandbox(t, "")
	if code := exitCode(func() { Ask(msg.MsgUnknown("me")) }); code != 1 {
		t.Errorf("no answer at all: exit code %d, want 1", code)
	}
}

func TestPromptsOnlyAtATerminal(t *testing.T) {
	_, errOut := sandbox(t, "y\n\n")
	Ask(msg.MsgUnknown("me"))
	if errOut.Len() != 0 {
		t.Errorf("input isn't a terminal, but the prompt was printed: %q", errOut)
	}
	stdinTTY = func() bool { return true }
	Confirm()
	if got, want := errOut.String(), "[Y/n] "; got != want {
		t.Errorf("prompt %q, want %q", got, want)
	}
	_, errOut = sandbox(t, "y\n")
	stdinTTY = func() bool { return true }
	Ask(msg.MsgUnknown("me"))
	if got, want := errOut.String(), "msg_unknown name=me [y/N] "; got != want {
		t.Errorf("prompt %q, want %q", got, want)
	}
}

func TestStdoutIsForData(t *testing.T) {
	out, _ := sandbox(t, "")
	_, _ = Stdout().Write([]byte("a table\n"))
	if out.String() != "a table\n" {
		t.Errorf("got %q", out)
	}
}
