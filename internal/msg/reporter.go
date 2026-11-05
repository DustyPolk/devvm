package msg

import (
	"context"
	"io"
	"os/exec"
)

// A Reporter is what devvm's packages need of the run's UI: where their
// messages go, the questions they ask and whether they can, the status
// line, the run log, output that isn't a message, and the run's cleanup
// stack. internal/ui's UI is the one devvm runs with, which internal/cli
// gives each package that embeds or takes a Reporter; tests give them a
// Fake.
type Reporter interface {
	// Info says what devvm did and Say prints a line, both on stdout; Note
	// prints a line and Warn a warning, both on stderr.
	Info(M)
	Say(M)
	Note(M)
	Warn(M)
	// Status shows a status line until stop.
	Status(label M) (stop func())

	// Ask asks a yes-or-no question, no by default ([y/N]); Confirm one
	// that's yes by default ([Y/n]); Answer asks for a value, "" for
	// Enter.
	Ask(M) bool
	Confirm(q ...M) bool
	Answer(prompt M) string
	// StdinIsTerminal reports whether a question can be asked,
	// StdoutIsTerminal whether data output goes to a terminal, and CanPick
	// whether a question with a default to fall back on is shown.
	StdinIsTerminal() bool
	StdoutIsTerminal() bool
	CanPick() bool

	// LogNote says where the run log is, once a run; LogWriter is the run
	// log, for lines that aren't messages; LoggedCmd runs a command into
	// it; WithSteps numbers the logged commands of a phase of so many
	// steps.
	LogNote()
	LogWriter() io.Writer
	LoggedCmd(ctx context.Context, step M, cmd *exec.Cmd) error
	WithSteps(ctx context.Context, total int) context.Context
	// Stdout is where data output goes, Stderr output on stderr that
	// isn't a message.
	Stdout() io.Writer
	Stderr() io.Writer

	// AddCleanup adds to the run's cleanup stack; Interrupted reports
	// whether the run was interrupted.
	AddCleanup(f func(context.Context)) (remove func())
	Interrupted() bool
}

var _ Reporter = (*Fake)(nil)

// A Fake is a Reporter for tests. Its zero value is quiet: it prints
// nothing, has no terminal, answers no to Ask and Confirm and "" to
// Answer, drops the run log and data output, runs a logged command as
// it is, its output dropped, counts no steps, keeps no cleanups and
// isn't interrupted. Its fields change that: each On func, when set, is
// given what its method is (a Recorder's Add records a kind of
// message), and the rest stand for the terminal, the writers and an
// interrupt. A test may change them after giving it to what it tests.
type Fake struct {
	OnInfo, OnSay, OnNote, OnWarn func(M)
	// OnAny is given every message, whatever its kind, after its kind's
	// On func.
	OnAny func(M)
	// OnConfirm is given Confirm's message, the zero M for none.
	OnAsk, OnConfirm func(M) bool
	OnAnswer         func(M) string
	// OnCleanup is AddCleanup.
	OnCleanup func(f func(context.Context)) (remove func())
	// Terminal is StdinIsTerminal, StdoutTerminal StdoutIsTerminal and
	// Picks CanPick.
	Terminal, StdoutTerminal, Picks bool
	// Interrupt is Interrupted.
	Interrupt bool
	// Out is Stdout, Err Stderr and Log LogWriter: nil drops what's
	// written.
	Out, Err, Log io.Writer
}

// Info gives m to OnInfo and OnAny.
func (f *Fake) Info(m M) { f.say(f.OnInfo, m) }

// Say gives m to OnSay and OnAny.
func (f *Fake) Say(m M) { f.say(f.OnSay, m) }

// Note gives m to OnNote and OnAny.
func (f *Fake) Note(m M) { f.say(f.OnNote, m) }

// Warn gives m to OnWarn and OnAny.
func (f *Fake) Warn(m M) { f.say(f.OnWarn, m) }

// Status shows nothing.
func (f *Fake) Status(M) func() { return func() {} }

// Ask is OnAsk's answer, no without one.
func (f *Fake) Ask(m M) bool {
	return f.OnAsk != nil && f.OnAsk(m)
}

// Confirm is OnConfirm's answer, no without one.
func (f *Fake) Confirm(q ...M) bool {
	var m M
	if len(q) > 0 {
		m = q[0]
	}
	return f.OnConfirm != nil && f.OnConfirm(m)
}

// Answer is OnAnswer's answer, "" without one.
func (f *Fake) Answer(prompt M) string {
	if f.OnAnswer == nil {
		return ""
	}
	return f.OnAnswer(prompt)
}

// StdinIsTerminal is Terminal.
func (f *Fake) StdinIsTerminal() bool { return f.Terminal }

// StdoutIsTerminal is StdoutTerminal.
func (f *Fake) StdoutIsTerminal() bool { return f.StdoutTerminal }

// CanPick is Picks.
func (f *Fake) CanPick() bool { return f.Picks }

// LogNote says nothing.
func (f *Fake) LogNote() {}

// LogWriter is Log, or nowhere.
func (f *Fake) LogWriter() io.Writer { return orDiscard(f.Log) }

// LoggedCmd runs cmd as it is.
func (f *Fake) LoggedCmd(_ context.Context, _ M, cmd *exec.Cmd) error { return cmd.Run() }

// WithSteps returns ctx.
func (f *Fake) WithSteps(ctx context.Context, _ int) context.Context { return ctx }

// Stdout is Out, or nowhere.
func (f *Fake) Stdout() io.Writer { return orDiscard(f.Out) }

// Stderr is Err, or nowhere.
func (f *Fake) Stderr() io.Writer { return orDiscard(f.Err) }

// AddCleanup gives c to OnCleanup; without one it keeps nothing.
func (f *Fake) AddCleanup(c func(context.Context)) func() {
	if f.OnCleanup == nil {
		return func() {}
	}
	return f.OnCleanup(c)
}

// Interrupted is Interrupt.
func (f *Fake) Interrupted() bool { return f.Interrupt }

// say gives m to on, then OnAny, each when it's set.
func (f *Fake) say(on func(M), m M) {
	for _, g := range []func(M){on, f.OnAny} {
		if g != nil {
			g(m)
		}
	}
}

func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
