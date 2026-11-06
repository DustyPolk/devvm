//go:build e2e

// Package e2e is devvm's end-to-end tests in Go: real VMs, in the sandbox
// tests/e2e.sh uses, a section at a time. TestE2E is the run; its flags
// (-e2e.only, -e2e.slots…) are tests/e2e.sh's options. Each section is a
// file of its own, listed in sections; tests/e2e.sh runs the sections not
// yet here and hands the rest to TestE2E. Run it from this folder, where
// go test streams its output:
//
//	cd tests/e2e && go test -tags e2e -count=1 -timeout 0 -run '^TestE2E$' -args -e2e.only=gc
package e2e

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// TestMain registers the run's flags on the command line's flag set, so
// go test's -args reach them; TestE2E reads them back (readOptions).
func TestMain(m *testing.M) {
	registerFlags(flag.CommandLine)
	os.Exit(m.Run())
}

// TestE2E is the run: the chosen sections, in a slot's sandbox, or spread
// over -e2e.slots of them. It fails when a check fails or a section ends
// early, as tests/e2e.sh does.
func TestE2E(t *testing.T) {
	if d := flag.Lookup("test.timeout").Value.String(); d != "0s" {
		t.Fatalf("e2e: run it with -timeout 0 (go test's %s would end a long section mid-check)", d)
	}
	o, err := readOptions(flag.CommandLine, sections())
	if err != nil {
		t.Fatalf("e2e: %v", err)
	}
	ctx, stop := signal.NotifyContext(t.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	out := newOutput(os.Stdout)
	var ok bool
	if o.slots > 1 {
		ok, err = runSlots(ctx, out, o)
	} else {
		ok, err = runSlot(ctx, out, o)
	}
	if err != nil {
		out.line("e2e: %v", err)
		ok = false
	}
	if ctx.Err() != nil && t.Context().Err() == nil {
		out.line("e2e: interrupted")
		ok = false
	}
	if !ok {
		t.Fail()
	}
}

// runSlot is one slot's run: its sandbox set up, the chosen sections run,
// then everything the run made deleted.
func runSlot(ctx context.Context, out *output, o options) (bool, error) {
	sb, err := newSandbox(o)
	if err != nil {
		return false, err
	}
	if err := sb.checkTools(o.sel); err != nil {
		return false, err
	}
	if err := sb.lock(); err != nil {
		return false, err
	}
	defer sb.unlock()
	// The cleanup outlives the run's context: an interrupted run still
	// deletes what it made.
	defer sb.cleanup(context.WithoutCancel(ctx), out)
	if err := sb.setup(ctx, out); err != nil {
		return false, err
	}
	r := newRunner(ctx, out, sb, o)
	defer r.stop()
	for _, sec := range sections() {
		if !o.wants(sec.name) || ctx.Err() != nil {
			continue
		}
		r.section(sec)
	}
	return r.finish(time.Now()), nil
}
