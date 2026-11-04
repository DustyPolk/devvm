package ui

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

// env is an environment for a UI's getenv.
type env map[string]string

func (e env) get(k string) string { return e[k] }

// testUI returns a UI on buffers, with input in, an empty environment and
// its run logs in a temporary folder.
func testUI(t *testing.T, in string) (u *UI, out, errOut *bytes.Buffer) {
	t.Helper()
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	u = New(strings.NewReader(in), out, errOut, env{}.get)
	// Plain, whatever FORCE_COLOR says, and messages as their names.
	u.outStyle, u.errStyle = style{names: true}, style{names: true}
	logs := filepath.Join(t.TempDir(), "logs", "runs")
	u.runLogs = func() (string, error) { return logs, nil }
	u.readPassword = func() (string, error) { return "", errors.New("no terminal") }
	t.Cleanup(func() {
		u.stopStatus()
		if u.log.f != nil {
			_ = u.log.f.Close()
		}
	})
	return u, out, errOut
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
	t.Parallel()
	m := msg.MsgUnknown("x")
	for _, tc := range []struct {
		name          string
		print         func(*UI, msg.M)
		stdout, stder string
	}{
		{"say", (*UI).Say, "msg_unknown name=x\n", ""},
		{"info", (*UI).Info, "==> msg_unknown name=x\n", ""},
		{"note", (*UI).Note, "", "msg_unknown name=x\n"},
		{"warn", (*UI).Warn, "", "devvm: msg_unknown name=x\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, out, errOut := testUI(t, "")
			tc.print(u, m)
			if out.String() != tc.stdout || errOut.String() != tc.stder {
				t.Errorf("stdout %q, stderr %q; want %q, %q", out, errOut, tc.stdout, tc.stder)
			}
			// With DataOnly, every message goes to stderr.
			u, out, errOut = testUI(t, "")
			u.DataOnly()
			tc.print(u, m)
			if out.Len() != 0 || errOut.String() != tc.stdout+tc.stder {
				t.Errorf("DataOnly: stdout %q, stderr %q; want nothing, %q", out, errOut, tc.stdout+tc.stder)
			}
		})
	}
}

func TestWarnAndDiePrefixEveryLine(t *testing.T) {
	if got, want := prefixLines("devvm: ", "one\ntwo"), "devvm: one\ndevvm: two\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	u, _, errOut := testUI(t, "")
	u.errStyle = style{}
	m := msg.MsgUnknown("a")
	u.Warn(m)
	if got, want := errOut.String(), "devvm: "+m.Text()+"\n"; got != want {
		t.Errorf("warn: got %q, want %q", got, want)
	}
}

func TestDiePrintsAndExits1(t *testing.T) {
	t.Parallel()
	u, out, errOut := testUI(t, "")
	if code := exitCode(func() { u.Die(msg.MsgUnknown("VM")) }); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if got, want := errOut.String(), "devvm: msg_unknown name=VM\n"; got != want || out.Len() != 0 {
		t.Errorf("stderr %q, stdout %q; want %q and nothing", got, out, want)
	}
	// Interrupted, it dies quietly.
	errOut.Reset()
	u.interrupted.Store(true)
	if code := exitCode(func() { u.Die(msg.MsgUnknown("VM")) }); code != 1 || errOut.Len() != 0 {
		t.Errorf("interrupted: exit code %d, stderr %q; want 1 and nothing", code, errOut)
	}
}

func TestConfirm(t *testing.T) {
	t.Parallel()
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
		u, _, _ := testUI(t, tc.in)
		var q []msg.M
		if !tc.bare {
			q = append(q, msg.MsgUnknown("x"))
		}
		if got := u.Confirm(q...); got != tc.want {
			t.Errorf("Confirm with %q = %v, want %v", tc.in, got, tc.want)
		}
	}
	u, _, _ := testUI(t, "")
	if code := exitCode(func() { u.Confirm(msg.MsgUnknown("x")) }); code != 1 {
		t.Errorf("no answer at all: exit code %d, want 1", code)
	}
}

func TestAsk(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]bool{"y\n": true, "Yes\n": true, "n\n": false, "\n": false, "x\n": false} {
		u, _, _ := testUI(t, in)
		if got := u.Ask(msg.MsgUnknown("me")); got != want {
			t.Errorf("Ask with %q = %v, want %v", in, got, want)
		}
	}
	u, _, _ := testUI(t, "")
	if code := exitCode(func() { u.Ask(msg.MsgUnknown("me")) }); code != 1 {
		t.Errorf("no answer at all: exit code %d, want 1", code)
	}
}

func TestPromptsOnlyAtATerminal(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "y\n\n")
	u.Ask(msg.MsgUnknown("me"))
	if errOut.Len() != 0 {
		t.Errorf("input isn't a terminal, but the prompt was printed: %q", errOut)
	}
	u.stdinTTY = true
	u.Confirm()
	if got, want := errOut.String(), "[Y/n] "; got != want {
		t.Errorf("prompt %q, want %q", got, want)
	}
	u, _, errOut = testUI(t, "y\n")
	u.stdinTTY = true
	u.Ask(msg.MsgUnknown("me"))
	if got, want := errOut.String(), "msg_unknown name=me [y/N] "; got != want {
		t.Errorf("prompt %q, want %q", got, want)
	}
}

func TestStdoutIsForData(t *testing.T) {
	t.Parallel()
	u, out, _ := testUI(t, "")
	_, _ = u.Stdout().Write([]byte("a table\n"))
	if out.String() != "a table\n" {
		t.Errorf("got %q", out)
	}
}

func TestStderrIsForLogLines(t *testing.T) {
	t.Parallel()
	u, out, errOut := testUI(t, "")
	_, _ = u.Stderr().Write([]byte("a log line\n"))
	if errOut.String() != "a log line\n" || out.Len() != 0 {
		t.Errorf("got %q, stdout %q", errOut, out)
	}
}

// A secret is read without echo at a terminal, after the prompt; else as a
// line, with no prompt.
func TestSecret(t *testing.T) {
	t.Parallel()
	u, out, errOut := testUI(t, "ghp_x\r\nrest")
	if v := u.Secret(msg.TokenPrompt()); v != "ghp_x" || errOut.Len() != 0 {
		t.Errorf("piped: %q, stderr %q", v, errOut)
	}
	if v := u.Line(); v != "rest" {
		t.Errorf("the end of input: %q", v)
	}
	if v := u.Line(); v != "" {
		t.Errorf("after it: %q", v)
	}
	u.readPassword = func() (string, error) { return "ghp_y", nil }
	u.stdinTTY = true
	if v := u.Secret(msg.TokenPrompt()); v != "ghp_y" || errOut.String() != "token_prompt \n" || out.Len() != 0 {
		t.Errorf("terminal: %q, stderr %q", v, errOut)
	}
}

// At the end of the input, with no answer, a question fails as it does
// with --no-input: the question shown first unless its prompt was, then
// what answers it, or a terminal for one only a terminal answers.
func TestNoAnswer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ask  func(u *UI)
		tty  bool
		want string
	}{
		{func(u *UI) { u.Ask(msg.GCConfirm()) }, false, "gc_confirm\ndevvm: no_input answer=--force\n"},
		{func(u *UI) { u.Confirm(msg.NetAsk()) }, false, "net_ask\ndevvm: no_input answer=--network-policy or --no-network-policy\n"},
		{func(u *UI) { u.Answer(msg.SizeCpusPrompt("4")) }, false, "size_cpus_prompt cpus=4\ndevvm: no_input answer=devvm size -g\n"},
		{func(u *UI) { u.Ask(msg.DeleteAnyway()) }, false, "delete_anyway\ndevvm: no_input_terminal\n"},
		{func(u *UI) { u.Ask(msg.GCConfirm()) }, true, "gc_confirm [y/N] \ndevvm: no_input answer=--force\n"},
	} {
		u, _, errOut := testUI(t, "")
		u.stdinTTY = tc.tty
		if code := exitCode(func() { tc.ask(u) }); code != 1 || errOut.String() != tc.want {
			t.Errorf("exit %d, stderr %q, want 1 and %q", code, errOut, tc.want)
		}
	}
}

// With --no-input every question fails before anything is read, after
// showing it: naming what answers it, or a terminal for one only a
// terminal answers.
func TestNoInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		ask  func(u *UI)
		want string
	}{
		{func(u *UI) { u.Ask(msg.GCConfirm()) }, "gc_confirm\ndevvm: no_input answer=--force\n"},
		{func(u *UI) { u.Confirm(msg.NetAsk()) }, "net_ask\ndevvm: no_input answer=--network-policy or --no-network-policy\n"},
		{func(u *UI) { u.Secret(msg.TokenPrompt()) }, "token_prompt\ndevvm: no_input answer=--stdin\n"},
		{func(u *UI) { u.Ask(msg.DeleteAnyway()) }, "delete_anyway\ndevvm: no_input_terminal\n"},
	} {
		u, _, errOut := testUI(t, "y\n")
		u.SetFlags(Flags{NoInput: true})
		if code := exitCode(func() { tc.ask(u) }); code != 1 || errOut.String() != tc.want {
			t.Errorf("exit %d, stderr %q, want %q", code, errOut, tc.want)
		}
		if line := u.Line(); line != "y" {
			t.Errorf("the answer was read: %q left", line)
		}
	}
}
