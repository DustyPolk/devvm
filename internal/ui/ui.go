// Package ui is the only place devvm prints. A UI prints messages (Say,
// Info, Note, Warn, Die), asks questions (Ask, Confirm, Pick), runs
// commands into the run's log behind a status line (LoggedCmd), and keeps
// the run's cleanup stack. Main makes the one a run has: it recovers Die,
// runs the cleanups and handles Ctrl+C.
package ui

import (
	"bufio"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// UI is devvm's terminal for one run: its streams and whether each is a
// terminal, the run log and the status line over it, the cleanup stack,
// and whether the run was interrupted. Main makes the one a run has.
type UI struct {
	// Writes to stdout and stderr hold outMu, so the status line and
	// messages don't interleave mid-line.
	outMu          sync.Mutex
	stdout, stderr io.Writer
	stdin          *bufio.Reader

	stdinTTY, stdoutTTY, stderrTTY bool
	// termWidth is the terminal's width in columns, 0 when unknown.
	termWidth func() int
	// readPassword reads a line from the terminal without echoing it.
	readPassword func() (string, error)
	// rawInput puts the terminal devvm reads from into raw mode, returning
	// what undoes it.
	rawInput func() (restore func(), err error)
	// getenv reads DEVVM_VERBOSE and the terminal's character set.
	getenv func(string) string
	// runLogs returns the run logs' folder.
	runLogs func() (string, error)
	// interruptGrace is how long a command a signal ended waits for
	// devvm's own interrupt to be noticed.
	interruptGrace time.Duration

	interrupted atomic.Bool
	// commands is read-locked while a logged command runs, so an
	// interrupt can wait for the run's commands to end before cleaning up.
	commands sync.RWMutex
	cleanups struct {
		sync.Mutex
		next  int
		funcs []cleanup
	}
	log struct {
		sync.Mutex
		path  string
		f     *os.File
		noted bool
	}
	status struct {
		sync.Mutex
		s *statusLine
	}
}

// New returns a UI on these streams, none of them a terminal, with its run
// log in devvm's data folder.
func New(stdin io.Reader, stdout, stderr io.Writer) *UI {
	u := &UI{
		stdout: stdout, stderr: stderr, stdin: bufio.NewReader(stdin),
		termWidth: func() int { return 0 }, rawInput: rawTerminal,
		getenv: os.Getenv, runLogs: runLogs, interruptGrace: time.Second,
	}
	u.readPassword = u.readTerminal
	return u
}

// onTerminal returns the UI on devvm's own standard streams.
func onTerminal() *UI {
	u := New(os.Stdin, os.Stdout, os.Stderr)
	u.stdinTTY, u.stdoutTTY, u.stderrTTY = isTerminal(os.Stdin), isTerminal(os.Stdout), isTerminal(os.Stderr)
	u.termWidth = func() int {
		w, _, err := term.GetSize(fd(os.Stderr))
		if err != nil {
			return 0
		}
		return w
	}
	return u
}

// runLogs is the run logs' folder in devvm's folders.
func runLogs() (string, error) {
	d, err := paths.FromEnv()
	if err != nil {
		return "", err
	}
	return d.RunLogs(), nil
}

func fd(f *os.File) int { return int(f.Fd()) } //nolint:gosec // file descriptors fit in an int

func isTerminal(f *os.File) bool { return term.IsTerminal(fd(f)) }

// StdinIsTerminal reports whether devvm's input is a terminal, so a question
// can be asked.
func (u *UI) StdinIsTerminal() bool { return u.stdinTTY }

// StdoutIsTerminal reports whether data output goes to a terminal, where
// someone could see it.
func (u *UI) StdoutIsTerminal() bool { return u.stdoutTTY }

// Stdout is where data output goes: tables, values, help. Not messages: they
// go through Say and the rest.
func (u *UI) Stdout() io.Writer { return lockedWriter{u, u.stdout} }

// Stderr is where output that isn't a message goes on stderr: the log lines
// of a process devvm runs as itself in the background (devvm _proxy).
func (u *UI) Stderr() io.Writer { return lockedWriter{u, u.stderr} }

type lockedWriter struct {
	u *UI
	w io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.u.outMu.Lock()
	defer l.u.outMu.Unlock()
	return l.w.Write(p)
}

func (u *UI) write(w io.Writer, s string) {
	u.outMu.Lock()
	defer u.outMu.Unlock()
	_, _ = io.WriteString(w, s)
}

// Say prints the message to stdout.
func (u *UI) Say(m msg.M) { u.write(u.stdout, m.String()+"\n") }

// Info prints "==> " and the message to stdout: what devvm did.
func (u *UI) Info(m msg.M) { u.write(u.stdout, "==> "+m.String()+"\n") }

// Note prints the message to stderr.
func (u *UI) Note(m msg.M) { u.write(u.stderr, m.String()+"\n") }

// Warn prints the message to stderr, with "devvm: " before each line.
func (u *UI) Warn(m msg.M) { u.write(u.stderr, prefixLines("devvm: ", m.String())) }

func prefixLines(prefix, s string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(s, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

// Die prints the message like Warn and ends the run with exit code 1: it
// panics, and Main runs the cleanups and returns 1. Once the run is
// interrupted it prints nothing, as the failure is the interrupt's.
func (u *UI) Die(m msg.M) {
	if !u.Interrupted() {
		u.Warn(m)
	}
	exit(1)
}

// DieLogged is Die after printing the last 25 lines of the run's log, if it
// has one, without control characters.
func (u *UI) DieLogged(m msg.M) {
	if path := u.logPath(); path != "" && !u.Interrupted() {
		u.write(u.stderr, "--- "+msg.LogEnd(path).Line()+"\n"+printable(tailLines(path, 25))+"---\n")
	}
	u.Die(m)
}

// Ask asks a yes-or-no question, no by default: it's true only for an answer
// starting with y. No answer at all (the end of input) ends the run with
// exit code 1, so nothing goes ahead unanswered.
func (u *UI) Ask(m msg.M) bool {
	a := u.answer(m.String() + " [y/N] ")
	return strings.HasPrefix(a, "y") || strings.HasPrefix(a, "Y")
}

// Confirm asks a yes-or-no question, yes by default: it's true unless the
// answer starts with n. Without a message it prompts only "[Y/n]", after a
// question printed on its own. No answer at all ends the run like Ask.
func (u *UI) Confirm(q ...msg.M) bool {
	prompt := "[Y/n] "
	if len(q) > 0 {
		prompt = q[0].String() + " " + prompt
	}
	a := u.answer(prompt)
	return !strings.HasPrefix(a, "n") && !strings.HasPrefix(a, "N")
}

// readTerminal reads a line from the terminal without echoing it. A
// cleanup puts the terminal back if the run ends meanwhile.
func (u *UI) readTerminal() (string, error) {
	f := fd(os.Stdin)
	old, err := term.GetState(f)
	if err != nil {
		return "", err
	}
	remove := u.AddCleanup(func(context.Context) { _ = term.Restore(f, old) })
	defer remove()
	b, err := term.ReadPassword(f)
	return string(b), err
}

// Secret reads a value such as a token: in a terminal after the prompt on
// stderr, without echoing it; else a line of input, as Line does.
func (u *UI) Secret(prompt msg.M) string {
	if !u.stdinTTY {
		return u.Line()
	}
	u.write(u.stderr, prompt.String()+" ")
	v, err := u.readPassword()
	u.write(u.stderr, "\n")
	if err != nil {
		return ""
	}
	return v
}

// Line reads a line of input, without its line ending; at the end of the
// input, what's left of it.
func (u *UI) Line() string {
	line, _ := u.stdin.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

// answer prints the prompt to stderr when the input is a terminal, and
// reads a line.
func (u *UI) answer(prompt string) string {
	if u.stdinTTY {
		u.write(u.stderr, prompt)
	}
	line, err := u.stdin.ReadString('\n')
	if err != nil && line == "" {
		exit(1)
	}
	return strings.TrimRight(line, "\r\n")
}
