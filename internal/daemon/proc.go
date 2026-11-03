package daemon

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// A Proc is one of devvm's background processes, found by its pid file:
// the process that file names, while it runs and its arguments say it's
// that one (Is), so a process given the pid since, or a file left by one
// that ended, is never taken for it.
type Proc struct {
	// PIDFile is where its pid is kept, on a line of its own.
	PIDFile string
	// Is reports whether a process's arguments, its program first, are
	// this one's.
	Is func(args []string) bool
	// Args returns a running process's arguments (platform.ProcessArgs).
	Args func(pid int) ([]string, error)
	// Signal sends it a signal: Signal, or a test's.
	Signal func(pid int, sig syscall.Signal) error
}

// HasFlag reports whether args, its program first, run devvm's command
// cmd with flag's value val (-vm NAME).
func HasFlag(args []string, cmd, flag, val string) bool {
	if len(args) < 2 || args[1] != cmd {
		return false
	}
	i := slices.Index(args, flag)
	return i >= 0 && i+1 < len(args) && args[i+1] == val
}

// PID returns the process's pid while it runs.
func (p Proc) PID() (int, bool) {
	pid, ok := readLine(p.PIDFile)
	if !ok || !Alive(pid) {
		return 0, false
	}
	args, err := p.Args(pid)
	if err != nil || !p.Is(args) {
		return 0, false
	}
	return pid, true
}

// Running reports whether the process runs.
func (p Proc) Running() bool {
	_, ok := p.PID()
	return ok
}

// runs reports whether pid is still the process.
func (p Proc) runs(pid int) bool {
	cur, ok := p.PID()
	return ok && cur == pid
}

// Start starts c with start (Detach, or a test's), after removing the
// status file an earlier start left, and records its pid in the pid file.
func (p Proc) Start(start Starter, c Cmd) (int, error) {
	if c.Status != "" {
		if err := os.Remove(c.Status); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	}
	pid, err := start(c)
	if err != nil {
		return 0, err
	}
	return pid, writeLine(p.PIDFile, strconv.Itoa(pid))
}

// Stop stops the process, if it runs: SIGTERM, then after wait SIGKILL,
// then up to wait more, looking every twentieth of it. Each look reads the
// pid file and the process's arguments again, so a process given the pid
// since is never signalled. Once it's stopped its pid file goes, unless a
// start since has written another pid there. It reports whether none runs
// then.
func (p Proc) Stop(wait time.Duration) bool {
	pid, ok := p.PID()
	if !ok {
		return true
	}
	signal := p.Signal
	if signal == nil {
		signal = Signal
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		_ = signal(pid, sig)
		for range 20 {
			if !p.runs(pid) {
				if cur, ok := readLine(p.PIDFile); ok && cur == pid {
					_ = os.Remove(p.PIDFile)
				}
				return true
			}
			time.Sleep(wait / 20)
		}
	}
	return false
}

// Ready waits for the process pid, which Start started with status, to
// say it's ready there (WaitReady), while it runs as this one.
func (p Proc) Ready(status string, pid int, tick time.Duration, tries int) (string, bool) {
	return WaitReady(status, pid, func() bool { return p.runs(pid) }, tick, tries)
}

// writeLine writes s as a file's one line, mode 600, in a single write:
// a reader that finds no line end yet (readLine) has caught it part-way.
func writeLine(path, s string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // devvm's own run folder
	if err != nil {
		return err
	}
	_, err = f.WriteString(s + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// readLine reads the pid on a file's first line, written whole
// (writeLine): ok is false for none, a partial one or no pid.
func readLine(path string) (int, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own run folder
	if err != nil {
		return 0, false
	}
	first, _, whole := strings.Cut(string(b), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	return pid, whole && err == nil && pid > 0
}
