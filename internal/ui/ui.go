// Package ui is the only place devvm prints. It prints messages (Say, Info,
// Note, Warn, Die), asks questions (Ask, Confirm), runs commands into the
// run's log behind a status line (Logged), and ends the run: Main recovers
// Die, runs the cleanup stack and handles Ctrl+C.
package ui

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/term"

	"github.com/w3cj/devvm/internal/msg"
)

// The streams, swapped by the tests. Writes to stdout and stderr hold outMu,
// so the status line and messages don't interleave mid-line.
var (
	outMu  sync.Mutex
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
	stdin            = bufio.NewReader(os.Stdin)

	stdinTTY  = func() bool { return isTerminal(os.Stdin) }
	stdoutTTY = func() bool { return isTerminal(os.Stdout) }
	stderrTTY = func() bool { return isTerminal(os.Stderr) }
	// termWidth is the terminal's width in columns, 0 when unknown.
	termWidth = func() int {
		w, _, err := term.GetSize(fd(os.Stderr))
		if err != nil {
			return 0
		}
		return w
	}
)

func fd(f *os.File) int { return int(f.Fd()) } //nolint:gosec // file descriptors fit in an int

func isTerminal(f *os.File) bool { return term.IsTerminal(fd(f)) }

// StdinIsTerminal reports whether devvm's input is a terminal, so a question
// can be asked.
func StdinIsTerminal() bool { return stdinTTY() }

// StdoutIsTerminal reports whether data output goes to a terminal, where
// someone could see it.
func StdoutIsTerminal() bool { return stdoutTTY() }

// Stdout is where data output goes: tables, values, help. Not messages: they
// go through Say and the rest.
func Stdout() io.Writer { return lockedWriter{stdout} }

// Stderr is where output that isn't a message goes on stderr: the log lines
// of a process devvm runs as itself in the background (devvm _proxy).
func Stderr() io.Writer { return lockedWriter{stderr} }

type lockedWriter struct{ w io.Writer }

func (l lockedWriter) Write(p []byte) (int, error) {
	outMu.Lock()
	defer outMu.Unlock()
	return l.w.Write(p)
}

func write(w io.Writer, s string) {
	outMu.Lock()
	defer outMu.Unlock()
	_, _ = io.WriteString(w, s)
}

// Say prints the message to stdout.
func Say(m msg.M) { write(stdout, m.String()+"\n") }

// Info prints "==> " and the message to stdout: what devvm did.
func Info(m msg.M) { write(stdout, "==> "+m.String()+"\n") }

// Note prints the message to stderr.
func Note(m msg.M) { write(stderr, m.String()+"\n") }

// Warn prints the message to stderr, with "devvm: " before each line.
func Warn(m msg.M) { write(stderr, prefixLines("devvm: ", m.String())) }

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
func Die(m msg.M) {
	if !Interrupted() {
		Warn(m)
	}
	exit(1)
}

// DieLogged is Die after printing the last 25 lines of the run's log, if it
// has one, without control characters.
func DieLogged(m msg.M) {
	if path := logPath(); path != "" && !Interrupted() {
		write(stderr, "--- "+msg.LogEnd(path).Line()+"\n"+printable(tailLines(path, 25))+"---\n")
	}
	Die(m)
}

// Ask asks a yes-or-no question, no by default: it's true only for an answer
// starting with y. No answer at all (the end of input) ends the run with
// exit code 1, so nothing goes ahead unanswered.
func Ask(m msg.M) bool {
	a := answer(m.String() + " [y/N] ")
	return strings.HasPrefix(a, "y") || strings.HasPrefix(a, "Y")
}

// Confirm asks a yes-or-no question, yes by default: it's true unless the
// answer starts with n. Without a message it prompts only "[Y/n]", after a
// question printed on its own. No answer at all ends the run like Ask.
func Confirm(q ...msg.M) bool {
	prompt := "[Y/n] "
	if len(q) > 0 {
		prompt = q[0].String() + " " + prompt
	}
	a := answer(prompt)
	return !strings.HasPrefix(a, "n") && !strings.HasPrefix(a, "N")
}

// answer prints the prompt to stderr when the input is a terminal, and
// reads a line.
func answer(prompt string) string {
	if stdinTTY() {
		write(stderr, prompt)
	}
	line, err := stdin.ReadString('\n')
	if err != nil && line == "" {
		exit(1)
	}
	return strings.TrimRight(line, "\r\n")
}
