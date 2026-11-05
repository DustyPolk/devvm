package msg

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"slices"
	"testing"
)

// A zero Fake is quiet: it prints nothing, has no terminal, answers no
// and "", drops what's written, runs a logged command as it is, keeps no
// cleanup and isn't interrupted.
func TestFakeQuiet(t *testing.T) {
	var f Fake
	m := NoVM("/x")
	f.Info(m)
	f.Say(m)
	f.Note(m)
	f.Warn(m)
	f.LogNote()
	f.Status(m)()
	f.AddCleanup(func(context.Context) { t.Error("kept") })()
	if f.Ask(m) || f.Confirm(m) || f.Confirm() || f.Answer(m) != "" {
		t.Error("answered")
	}
	if f.StdinIsTerminal() || f.StdoutIsTerminal() || f.CanPick() || f.Interrupted() {
		t.Error("a terminal, the picker or an interrupt")
	}
	for _, w := range []io.Writer{f.Stdout(), f.Stderr(), f.LogWriter()} {
		if w != io.Discard {
			t.Errorf("writer %v", w)
		}
	}
	ctx := t.Context()
	if f.WithSteps(ctx, 3) != ctx {
		t.Error("steps counted")
	}
	if err := f.LoggedCmd(ctx, m, exec.CommandContext(ctx, "true")); err != nil {
		t.Errorf("LoggedCmd: %v", err)
	}
}

// A Fake's fields get what its methods are given, and stand for the
// terminal, the writers and an interrupt.
func TestFakeRecords(t *testing.T) {
	var kinds, all Recorder
	var asked []string
	var out, errs, log bytes.Buffer
	var cleanups int
	f := &Fake{
		OnInfo: kinds.Add, OnSay: kinds.Add, OnNote: kinds.Add, OnWarn: kinds.Add, OnAny: all.Add,
		OnAsk:     func(m M) bool { asked = append(asked, "ask "+m.Names()); return true },
		OnConfirm: func(m M) bool { asked = append(asked, "confirm "+m.Names()); return true },
		OnAnswer:  func(m M) string { asked = append(asked, "answer "+m.Names()); return "4" },
		OnCleanup: func(func(context.Context)) func() { cleanups++; return func() {} },
		Terminal:  true, StdoutTerminal: true, Picks: true, Interrupt: true,
		Out: &out, Err: &errs, Log: &log,
	}
	f.Info(NoVM("/i"))
	f.Say(NoVM("/s"))
	f.Note(NoVM("/n"))
	f.Warn(NoVM("/w"))
	want := []string{"no_vm dir=/i", "no_vm dir=/s", "no_vm dir=/n", "no_vm dir=/w"}
	if !slices.Equal(kinds.Names(), want) || !slices.Equal(all.Names(), want) {
		t.Errorf("kinds %q, all %q", kinds.Names(), all.Names())
	}
	if !f.Ask(NoVM("/a")) || !f.Confirm(NoVM("/c")) || f.Answer(NoVM("/v")) != "4" {
		t.Error("not answered")
	}
	if want := []string{"ask no_vm dir=/a", "confirm no_vm dir=/c", "answer no_vm dir=/v"}; !slices.Equal(asked, want) {
		t.Errorf("asked %q", asked)
	}
	f.AddCleanup(func(context.Context) {})
	if cleanups != 1 || !f.StdinIsTerminal() || !f.StdoutIsTerminal() || !f.CanPick() || !f.Interrupted() {
		t.Error("its fields aren't what it says")
	}
	if f.Stdout() != &out || f.Stderr() != &errs || f.LogWriter() != &log {
		t.Error("not its writers")
	}
}
