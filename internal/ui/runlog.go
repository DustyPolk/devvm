package ui

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/redact"
	"github.com/w3cj/devvm/internal/state"
)

// keepLogs is how many run logs are kept, the newest.
const keepLogs = 20

// streamWait is how long a streamed command's output is read after the
// command ends.
const streamWait = 2 * time.Second

// The output of the commands devvm runs (the backends', the build
// scripts') goes to a log, one per devvm run that has any, not the
// terminal: devvm prints its own steps behind a status line, and the end
// of the log when a step fails. Verbose (-v) also streams each line the
// log gets to stderr as it's written, without control characters, dim in
// a terminal, instead of the status line.

// Flags are the run's flags that every command takes and the UI acts
// on.
type Flags struct {
	Verbose bool // -v/--verbose: the run log streamed to stderr too
	NoInput bool // --no-input: no question asked; each fails, naming its answer
}

// SetFlags sets the run's -v/--verbose and --no-input.
func (u *UI) SetFlags(f Flags) {
	u.verboseOn.Store(f.Verbose)
	u.noInput.Store(f.NoInput)
}

// verbose reports whether u streams the run log (Flags.Verbose).
func verbose(u *UI) bool { return u.verboseOn.Load() }

// stream is the run log's copy on stderr while verbose: what's written to
// it goes to stderr printable and dim. A lead byte of a C1 control
// (0xc2) at a write's end is held for the next, so a control split
// across writes is still dropped.
type stream struct {
	u    *UI
	mu   sync.Mutex
	held []byte
}

func (s *stream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := slices.Concat(s.held, p)
	s.held = nil
	if n := len(b); n > 0 && b[n-1] == 0xc2 {
		b, s.held = b[:n-1], []byte{0xc2}
	}
	if text := msg.Printable(string(b)); text != "" {
		s.u.write(s.u.stderr, s.u.errStyle.paintLines(sgrDim, text))
	}
	return len(p), nil
}

// logTo is where a command's output goes: the run log, and while
// verbose, the stream too.
func logTo(u *UI, f *os.File) io.Writer {
	if !verbose(u) {
		return f
	}
	return io.MultiWriter(f, &stream{u: u})
}

// logPath is the run's log, "" before it's opened.
func (u *UI) logPath() string {
	u.log.Lock()
	defer u.log.Unlock()
	return u.log.path
}

// openLog opens the run's log, the first time in a run, in the run logs
// folder (owner-only), and deletes all but the newest logs there.
func (u *UI) openLog() *os.File {
	u.log.Lock()
	defer u.log.Unlock()
	if u.log.f != nil {
		return u.log.f
	}
	dir, err := u.runLogs()
	if err != nil {
		if m, ok := errors.AsType[msg.M](err); ok {
			u.Die(m)
		}
		u.Die(msg.RunLogFailed("", err.Error()).Because(err))
	}
	f, err := createLog(dir, time.Now(), os.Getpid())
	if err != nil {
		u.Die(msg.RunLogFailed(dir, err.Error()).Because(err))
	}
	u.log.f, u.log.path = f, f.Name()
	pruneLogs(dir, cmp.Or(u.log.keep, keepLogs))
	return f
}

// KeepRunLogs sets how many run logs are kept, the newest (retention's
// logs.runs); 0 is the default, keepLogs.
func (u *UI) KeepRunLogs(n int) {
	u.log.Lock()
	defer u.log.Unlock()
	u.log.keep = max(n, 0)
}

func createLog(dir string, now time.Time, pid int) (*os.File, error) {
	for _, d := range []string{filepath.Dir(dir), dir} {
		if err := state.PrivateDir(d); err != nil {
			return nil, err
		}
	}
	name := now.Format("20060102-150405") + "-" + strconv.Itoa(pid) + ".log"
	return os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // devvm's own folder
}

// pruneLogs deletes all but the newest keep logs in dir.
func pruneLogs(dir string, keep int) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	type log struct {
		path string
		mod  time.Time
	}
	var logs []log
	for _, f := range files {
		if fi, err := os.Stat(f); err == nil {
			logs = append(logs, log{f, fi.ModTime()})
		}
	}
	slices.SortFunc(logs, func(a, b log) int { return b.mod.Compare(a.mod) })
	for _, l := range logs[min(keep, len(logs)):] {
		_ = os.Remove(l.path)
	}
}

// LogNote says where the details of slow steps go, once per run.
func (u *UI) LogNote() {
	path := u.openLog().Name()
	u.log.Lock()
	noted := u.log.noted
	u.log.noted = true
	u.log.Unlock()
	if !noted {
		w, st := says(u)
		u.write(w, "    "+st.message(msg.LogDetails(path))+"\n")
	}
}

// LogWriter is where to send the output of a command run some other way
// than LoggedCmd (a backend's Exec, say): the run log, streamed to stderr
// too while verbose, through the run's one redacting writer, so a password
// split across writes is still found. The end of a line still to come is
// written at the next flush point (flushLog): a logged command's start,
// DieLogged and the run's end.
func (u *UI) LogWriter() io.Writer {
	f := u.openLog()
	u.log.Lock()
	defer u.log.Unlock()
	if u.log.w == nil {
		u.log.w = &redacting{w: logTo(u, f)}
	}
	return u.log.w
}

// flushLog writes what u's LogWriter holds of a line, redacted.
func flushLog(u *UI) {
	u.log.Lock()
	w := u.log.w
	u.log.Unlock()
	if w != nil {
		w.flush()
	}
}

// redactLineMax is as much of a line without its end as redacting holds: past it, the line is
// written as far as it goes.
const redactLineMax = 64 << 10

// redacting is a command's output on its way to the run log: it writes whole lines, each
// redacted (redact.String), so a password split across two writes is still found. A line ends at
// "\n" or "\r" (a progress bar's). flush writes the rest, once the command has ended.
type redacting struct {
	w   io.Writer
	mu  sync.Mutex
	buf []byte
}

func (r *redacting) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if i := bytes.LastIndexAny(r.buf, "\n\r"); i >= 0 {
		_, _ = io.WriteString(r.w, redact.String(string(r.buf[:i+1])))
		r.buf = slices.Clone(r.buf[i+1:])
	} else if len(r.buf) > redactLineMax {
		_, _ = io.WriteString(r.w, redact.String(string(r.buf)))
		r.buf = nil
	}
	return len(p), nil
}

func (r *redacting) flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.buf) > 0 {
		_, _ = io.WriteString(r.w, redact.String(string(r.buf)))
		r.buf = nil
	}
}

// steps counts the labelled steps in a phase (an image's build, say).
type steps struct {
	sync.Mutex
	n, total int
}

type stepsKey struct{}

// WithSteps starts a phase of total steps: each labelled LoggedCmd with the
// context returned is numbered on its status line ("[2/5] running base.sh")
// and in the log ("# [2/5] running base.sh"). A phase inside another keeps
// its own count.
func (u *UI) WithSteps(ctx context.Context, total int) context.Context {
	return context.WithValue(ctx, stepsKey{}, &steps{total: total})
}

// count numbers a labelled step in its phase, if any: its label for the
// status line, and its line for the log ("" outside a phase).
func count(ctx context.Context, step string) (label, logLine string) {
	s, _ := ctx.Value(stepsKey{}).(*steps)
	if s == nil || s.total <= 0 {
		return step, ""
	}
	s.Lock()
	defer s.Unlock()
	s.n++
	n := "[" + strconv.Itoa(s.n) + "/" + strconv.Itoa(s.total) + "]"
	if s.n <= s.total {
		label = n + " " + step
	} else {
		label = step
	}
	return label, "# " + n + " " + step + "\n"
}

// LoggedCmd runs cmd into the run log, with a status line in a terminal
// showing step. A step may be the zero msg.M: the status line then shows
// the command and its first argument. ctx carries the phase's step count.
// It returns the command's error; the caller dies with what failed
// (DieLogged).
func (u *UI) LoggedCmd(ctx context.Context, step msg.M, cmd *exec.Cmd) error {
	f := u.openLog()
	w := logTo(u, f)
	label := []part{{text: strings.Join(cmd.Args[:min(2, len(cmd.Args))], " ")}}
	logLine := ""
	if step.ID() != "" {
		var counted string
		shown := u.errStyle.line(step)
		counted, logLine = count(ctx, shown)
		// The count before the step, then the step, its values bold.
		label = append([]part{{text: strings.TrimSuffix(counted, shown)}}, u.errStyle.valueParts(step)...)
	}
	flushLog(u) // what LogWriter holds goes first
	_, _ = io.WriteString(w, logLine+redact.String("+ "+strings.Join(cmd.Args, " "))+"\n")
	out := &redacting{w: w}
	defer out.flush()
	cmd.Stdout, cmd.Stderr = out, out

	var s *statusLine
	if u.stderrTTY && !verbose(u) {
		s = u.startStatus(label)
	}
	start := time.Now()
	err := u.run(cmd)
	if s != nil {
		s.stop()
		if took := time.Since(start); err == nil && took >= doneAfter {
			u.write(u.stderr, u.doneLine(label, int(took.Seconds())))
		}
	}
	return err
}

func (u *UI) run(cmd *exec.Cmd) error {
	u.commands.RLock()
	defer u.commands.RUnlock()
	// Streamed output is copied through a pipe, which a process the
	// command left running (a VM's own) can hold open: once the command
	// ends, its output gets a moment, then the pipe is closed.
	if _, file := cmd.Stdout.(*os.File); !file && cmd.WaitDelay == 0 {
		cmd.WaitDelay = streamWait
	}
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	if killed(err) {
		u.awaitInterrupt(u.interruptGrace)
	}
	return err
}

// killed reports whether a command ended by a signal, or with the code a
// shell gives that (128 and over).
func killed(err error) bool {
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	return ok && (exitErr.ExitCode() == -1 || exitErr.ExitCode() >= 128)
}

// awaitInterrupt waits up to d for an interrupt. Ctrl+C reaches devvm and
// its command at once, and the command can end first: its failure isn't
// devvm's to report.
func (u *UI) awaitInterrupt(d time.Duration) {
	for deadline := time.Now().Add(d); !u.Interrupted() && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
}
