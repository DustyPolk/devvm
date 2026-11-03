package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// output waits for a process's output file to have want, and returns it.
func output(t *testing.T, path, want string) string {
	t.Helper()
	var got string
	for range 250 {
		b, _ := os.ReadFile(path) //nolint:gosec // the test's own file
		if got = string(b); strings.Contains(got, want) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return got
}

// A detached process leads a group (and session) of its own, gets no
// input, writes to the file it's given, and gets its own status file's
// name in its environment, never one devvm was given.
func TestDetach(t *testing.T) {
	t.Setenv(StatusEnv, "/devvm's/own")
	out, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	script := `read x; echo "ran $?"; echo "status=$` + StatusEnv + `"; echo "group=$(ps -o pgid= -p $$ | tr -d ' ')"; exec sleep 30`
	h, err := Spawn(Cmd{Path: "/bin/sh", Args: []string{"-c", script}, Out: out, Status: "/s"})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop(time.Second)
	got := output(t, out.Name(), "group=")
	want := "ran 1\nstatus=/s\ngroup=" + strconv.Itoa(h.PID()) + "\n"
	if got != want {
		t.Errorf("output %q, want %q", got, want)
	}
	if !h.Running() || !Alive(h.PID()) {
		t.Error("not running")
	}
	// Without a status file, none in its environment.
	pid, err := Detach(Cmd{Path: "/bin/sh", Args: []string{"-c", `echo "status=$` + StatusEnv + `."`}, Out: out})
	if err != nil || pid <= 0 {
		t.Fatal(pid, err)
	}
	if got := output(t, out.Name(), "status=."); !strings.HasSuffix(got, "status=.\n") {
		t.Errorf("output %q", got)
	}
	// It's reaped once it ends: never alive after.
	for range 250 {
		if !Alive(pid) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if Alive(pid) {
		t.Errorf("%d ended but looks alive", pid)
	}
	if _, err := Detach(Cmd{Path: filepath.Join(t.TempDir(), "none")}); err == nil {
		t.Error("a program that isn't there: started")
	}
}

// A Handle stops its process with SIGTERM, or SIGKILL when that doesn't
// end it; one that has ended gets no signal; a nil one is none.
func TestHandleStop(t *testing.T) {
	h, err := Spawn(Cmd{Path: "sleep", Args: []string{"30"}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if !h.Stop(5*time.Second) || h.Running() || time.Since(start) > 4*time.Second {
		t.Errorf("SIGTERM: stopped %v after %v", !h.Running(), time.Since(start))
	}
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	stubborn, err := Spawn(Cmd{Path: "/bin/sh", Args: []string{"-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}, Out: f})
	if err != nil {
		t.Fatal(err)
	}
	output(t, out, "ready")
	if !stubborn.Stop(100 * time.Millisecond) {
		t.Error("SIGKILL: still running")
	}
	select {
	case <-stubborn.Done():
	default:
		t.Error("not done")
	}
	if !stubborn.Stop(time.Second) {
		t.Error("an ended one: not stopped")
	}
	var none *Handle
	if !none.Stop(time.Second) {
		t.Error("none: not stopped")
	}
}

// Signal and Alive never take a pid below 1, which names a group of
// processes, or every one.
func TestSignalRefusesGroups(t *testing.T) {
	for _, pid := range []int{0, -1, -os.Getpid()} {
		if Alive(pid) {
			t.Errorf("Alive(%d)", pid)
		}
		if err := Signal(pid, syscall.SIGKILL); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("Signal(%d) = %v", pid, err)
		}
	}
	if !Alive(os.Getpid()) || Signal(os.Getpid(), 0) != nil {
		t.Error("this test isn't alive")
	}
}

func TestHasFlag(t *testing.T) {
	for _, c := range []struct {
		args []string
		ok   bool
	}{
		{[]string{"/devvm", "_proxy", "-listen", "x", "-vm", "v"}, true},
		{[]string{"/devvm", "_proxy", "-vm", "w"}, false},
		{[]string{"/devvm", "_links", "-vm", "v"}, false},
		{[]string{"/devvm", "_proxy", "-vm"}, false},
		{[]string{"/devvm", "_proxy"}, false},
		{[]string{"/devvm"}, false},
		{nil, false},
	} {
		if got := HasFlag(c.args, "_proxy", "-vm", "v"); got != c.ok {
			t.Errorf("HasFlag(%q) = %v", c.args, got)
		}
	}
}

// proc is a Proc whose processes are the test's sleeps, each one devvm
// _x -vm v while it's in args.
func proc(t *testing.T) (Proc, Starter, map[int]bool) {
	t.Helper()
	ours := map[int]bool{}
	p := Proc{
		PIDFile: filepath.Join(t.TempDir(), "v.pid"),
		Is:      func(args []string) bool { return HasFlag(args, "_x", "-vm", "v") },
		Args: func(pid int) ([]string, error) {
			if !ours[pid] {
				return []string{"/usr/bin/other"}, nil
			}
			return []string{"/devvm", "_x", "-vm", "v"}, nil
		},
	}
	start := func(c Cmd) (int, error) {
		h, err := Spawn(Cmd{Path: c.Path, Args: c.Args, Out: c.Out, Status: c.Status})
		if err != nil {
			return 0, err
		}
		t.Cleanup(func() { h.Stop(time.Second) })
		ours[h.PID()] = true
		return h.PID(), nil
	}
	return p, start, ours
}

// A Proc is the process its pid file names while that runs with its
// arguments: not a pid another program has now (this test's), a stale
// file whose process ended, a pid that names a group, or a file half
// written.
func TestProcPID(t *testing.T) {
	p, start, ours := proc(t)
	if _, ok := p.PID(); ok {
		t.Error("a process before any started")
	}
	pid, err := p.Start(start, Cmd{Path: "sleep", Args: []string{"30"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := p.PID(); !ok || got != pid || !p.Running() {
		t.Errorf("PID = %d, %v; want %d", got, ok, pid)
	}
	if b, err := os.ReadFile(p.PIDFile); err != nil || string(b) != strconv.Itoa(pid)+"\n" {
		t.Errorf("pid file %q, %v", b, err)
	}
	if fi, err := os.Stat(p.PIDFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("pid file: %v, %v", fi.Mode(), err)
	}
	delete(ours, pid)
	if _, ok := p.PID(); ok {
		t.Error("a process with other arguments counts")
	}
	ended, err := p.Start(start, Cmd{Path: "true"})
	if err != nil {
		t.Fatal(err)
	}
	for range 250 {
		if !Alive(ended) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, pidFile := range []string{strconv.Itoa(ended) + "\n", strconv.Itoa(os.Getpid()) + "\n", "0\n", "-1\n", "x\n", "", strconv.Itoa(pid)} {
		ours[pid] = true
		if err := os.WriteFile(p.PIDFile, []byte(pidFile), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, ok := p.PID(); ok {
			t.Errorf("pid file %q: %d", pidFile, got)
		}
	}
}

// Stop signals only the process the pid file names while it runs as this
// one: SIGTERM, then SIGKILL; it removes the pid file once it has stopped
// it, unless a start since wrote another pid there; one it can't stop is
// reported.
func TestProcStop(t *testing.T) {
	p, start, _ := proc(t)
	var signalled []string
	p.Signal = func(pid int, sig syscall.Signal) error {
		signalled = append(signalled, sig.String())
		return Signal(pid, sig)
	}
	// None, and a reused pid: nothing signalled.
	if !p.Stop(time.Second) || len(signalled) != 0 {
		t.Errorf("none: %v", signalled)
	}
	if err := os.WriteFile(p.PIDFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !p.Stop(time.Second) || len(signalled) != 0 {
		t.Errorf("a reused pid: %v", signalled)
	}
	if _, err := os.Stat(p.PIDFile); err != nil {
		t.Errorf("another program's pid file went: %v", err)
	}
	if _, err := p.Start(start, Cmd{Path: "sleep", Args: []string{"30"}}); err != nil {
		t.Fatal(err)
	}
	if !p.Stop(2*time.Second) || p.Running() || !slices.Equal(signalled, []string{"terminated"}) {
		t.Errorf("stopped: %v, signalled %v", !p.Running(), signalled)
	}
	if _, err := os.Stat(p.PIDFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("its pid file stayed: %v", err)
	}
	// One that ignores SIGTERM is killed.
	signalled = nil
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := p.Start(start, Cmd{Path: "/bin/sh", Args: []string{"-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}, Out: f}); err != nil {
		t.Fatal(err)
	}
	output(t, out, "ready")
	if !p.Stop(200*time.Millisecond) || !slices.Equal(signalled, []string{"terminated", "killed"}) {
		t.Errorf("signalled %v", signalled)
	}
	// One whose signals never arrive is reported.
	if _, err := p.Start(start, Cmd{Path: "sleep", Args: []string{"30"}}); err != nil {
		t.Fatal(err)
	}
	p.Signal = func(int, syscall.Signal) error { return nil }
	if p.Stop(20 * time.Millisecond) {
		t.Error("stopped one that never got a signal")
	}
	if !p.Running() {
		t.Error("it ended")
	}
}

// A process that's ending still has its files, a listening socket among
// them, after its arguments can no longer be read (macOS's kern.procargs2
// fails first): Stop waits until it's gone, so its port is free when Stop
// returns.
func TestProcStopWaitsForTheEnd(t *testing.T) {
	p, start, ours := proc(t)
	ending := map[int]bool{}
	args := p.Args
	p.Args = func(pid int) ([]string, error) {
		if ending[pid] {
			return nil, errors.New("ending")
		}
		return args(pid)
	}
	p.Signal = func(pid int, sig syscall.Signal) error {
		ending[pid] = true
		return Signal(pid, sig)
	}
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	pid, err := p.Start(start, Cmd{Path: "/bin/sh", Args: []string{"-c", `trap "sleep 0.5; exit 0" TERM; echo ready; while :; do sleep 0.05; done`}, Out: f})
	if err != nil {
		t.Fatal(err)
	}
	output(t, out, "ready")
	if !ours[pid] {
		t.Fatal("not the test's")
	}
	if !p.Stop(2 * time.Second) {
		t.Fatal("not stopped")
	}
	if Alive(pid) {
		t.Error("Stop returned while the process was still ending")
	}
}

// SIGKILL goes only to a process whose arguments still say it's the one:
// one whose arguments can't be read after SIGTERM (still ending, or
// another given the pid) is waited for, never killed.
func TestProcStopKillsOnlyItsOwn(t *testing.T) {
	p, start, _ := proc(t)
	unreadable := map[int]bool{}
	args := p.Args
	p.Args = func(pid int) ([]string, error) {
		if unreadable[pid] {
			return nil, errors.New("unreadable")
		}
		return args(pid)
	}
	var signalled []string
	p.Signal = func(pid int, sig syscall.Signal) error {
		signalled = append(signalled, sig.String())
		unreadable[pid] = true
		return Signal(pid, sig)
	}
	out := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(out) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	pid, err := p.Start(start, Cmd{Path: "/bin/sh", Args: []string{"-c", `trap "" TERM; echo ready; while :; do sleep 0.05; done`}, Out: f})
	if err != nil {
		t.Fatal(err)
	}
	output(t, out, "ready")
	if p.Stop(200 * time.Millisecond) {
		t.Error("stopped, though it ignores SIGTERM")
	}
	if !slices.Equal(signalled, []string{"terminated"}) {
		t.Errorf("signalled %v, want only SIGTERM", signalled)
	}
	if !Alive(pid) {
		t.Error("killed")
	}
}

// A start removes the status file an earlier start left, so readiness is
// this start's alone; the process says it's ready there, with its pid,
// and is waited for: ready while it runs, never once it has ended, nor
// for another process's status, a half-written one or none.
func TestReady(t *testing.T) {
	p, start, _ := proc(t)
	status := filepath.Join(t.TempDir(), "v.status")
	if err := WriteStatus(status, 1, "old"); err != nil {
		t.Fatal(err)
	}
	script := `echo "$` + StatusEnv + `"; sleep 0.2; printf '%s\n%s\n' $$ 127.0.0.1:3128 > "$` + StatusEnv + `"; exec sleep 30`
	pid, err := p.Start(start, Cmd{Path: "/bin/sh", Args: []string{"-c", script}, Status: status})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(status); err == nil {
		t.Error("an earlier start's status stayed")
	}
	if got, ok := p.Ready(status, pid, 20*time.Millisecond, 100); !ok || got != "127.0.0.1:3128" {
		b, err := os.ReadFile(status) //nolint:gosec // the test's own file
		t.Errorf("Ready = %q, %v; status %q, %v; pid %d, running %v", got, ok, b, err, pid, p.Running())
	}
	p.Stop(time.Second)
	if _, ok := p.Ready(status, pid, time.Millisecond, 10); ok {
		t.Error("ready once it ended")
	}
	runs := func() bool { return true }
	for _, s := range []string{"", "1\nx\n", strconv.Itoa(pid) + "\n", strconv.Itoa(pid) + "\nx"} {
		if err := os.WriteFile(status, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		if got, ok := WaitReady(status, pid, runs, time.Millisecond, 3); ok {
			t.Errorf("status %q: ready, %q", s, got)
		}
	}
	if err := os.Remove(status); err != nil {
		t.Fatal(err)
	}
	if err := WriteStatus(status, pid, "a\nb"); err != nil {
		t.Fatal(err)
	}
	if got, ok := WaitReady(status, pid, runs, time.Millisecond, 3); !ok || got != "a b" {
		t.Errorf("Ready = %q, %v", got, ok)
	}
	if fi, err := os.Stat(status); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("status file: %v, %v", fi.Mode(), err)
	}
}

// Ready writes the status file its start named, with its own pid, and
// takes the name out of its environment; without one it does nothing.
func TestReadyWrites(t *testing.T) {
	status := filepath.Join(t.TempDir(), "s")
	t.Setenv(StatusEnv, status)
	if err := Ready("127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if got, ok := readStatus(status, os.Getpid()); !ok || got != "127.0.0.1:1" {
		t.Errorf("status %q, %v", got, ok)
	}
	if v, ok := os.LookupEnv(StatusEnv); ok {
		t.Errorf("%s=%s stayed", StatusEnv, v)
	}
	if err := os.Remove(status); err != nil {
		t.Fatal(err)
	}
	if err := Ready("again"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(status); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("written without a name: %v", err)
	}
	t.Setenv(StatusEnv, filepath.Join(t.TempDir(), "no", "such", "folder"))
	if err := Ready("x"); err == nil {
		t.Error("an unwritable status file: no error")
	}
}
