package ui

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// keepLogs is how many run logs are kept, the newest.
const keepLogs = 20

// The output of the commands devvm runs (the backends', the build scripts')
// goes to a log, one per devvm run that has any, not the terminal: devvm
// prints its own steps, and the end of the log when a step fails.
// DEVVM_VERBOSE=1 prints it all instead.
var runLog struct {
	sync.Mutex
	path  string
	f     *os.File
	noted bool
}

func verbose() bool { return os.Getenv("DEVVM_VERBOSE") != "" }

// logPath is the run's log, "" before it's opened.
func logPath() string {
	runLog.Lock()
	defer runLog.Unlock()
	return runLog.path
}

// openLog opens the run's log, the first time in a run, in the run logs
// folder (owner-only), and deletes all but the newest logs there.
func openLog() *os.File {
	runLog.Lock()
	defer runLog.Unlock()
	if runLog.f != nil {
		return runLog.f
	}
	d, err := paths.FromEnv()
	if err != nil {
		if m, ok := errors.AsType[msg.M](err); ok {
			Die(m)
		}
		Die(msg.RunLogFailed("", err.Error()))
	}
	dir := d.RunLogs()
	f, err := createLog(dir, time.Now(), os.Getpid())
	if err != nil {
		Die(msg.RunLogFailed(dir, err.Error()))
	}
	runLog.f, runLog.path = f, f.Name()
	pruneLogs(dir, keepLogs)
	return f
}

func createLog(dir string, now time.Time, pid int) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	for _, d := range []string{filepath.Dir(dir), dir} {
		if err := os.Chmod(d, 0o700); err != nil { //nolint:gosec // folders: owner-only with search

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
func LogNote() {
	if verbose() {
		return
	}
	path := openLog().Name()
	runLog.Lock()
	noted := runLog.noted
	runLog.noted = true
	runLog.Unlock()
	if !noted {
		write(stdout, "    "+msg.LogDetails(path).String()+"\n")
	}
}

// LogWriter is where to send the output of a command run some other way
// than Logged (a backend's Exec, say): the run log, or with DEVVM_VERBOSE
// the terminal.
func LogWriter() io.Writer {
	if verbose() {
		return lockedWriter{stderr}
	}
	return openLog()
}

// proxyKey is a VM's proxy key in a proxy URL, which lets a process act as
// that VM. It's kept out of the run log.
var proxyKey = regexp.MustCompile(`devvm:[0-9a-f]{32}@`)

func redact(s string) string { return proxyKey.ReplaceAllString(s, "devvm:<key>@") }

// steps counts the labelled steps in a phase (an image's build, say).
type steps struct {
	sync.Mutex
	n, total int
}

type stepsKey struct{}

// WithSteps starts a phase of total steps: each labelled Logged with the
// context returned is numbered on its status line ("[2/5] running base.sh")
// and in the log ("# [2/5] running base.sh"). A phase inside another keeps
// its own count.
func WithSteps(ctx context.Context, total int) context.Context {
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

// Logged runs argv into the run log, with a status line in a terminal
// showing step. A step may be the zero msg.M: the status line then shows
// the command and its first argument. The command gets no input. It returns
// the command's error; the caller dies with what failed (DieLogged).
func Logged(ctx context.Context, step msg.M, argv ...string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // devvm's own commands
	return LoggedCmd(ctx, step, cmd)
}

// LoggedCmd is Logged for a command that needs more than its arguments: its
// input, environment or folder. ctx carries the phase's step count.
func LoggedCmd(ctx context.Context, step msg.M, cmd *exec.Cmd) error {
	if verbose() {
		cmd.Stdout, cmd.Stderr = Stdout(), lockedWriter{stderr}
		return run(cmd)
	}
	f := openLog()
	label := strings.Join(cmd.Args[:min(2, len(cmd.Args))], " ")
	logLine := ""
	if step.ID() != "" {
		label, logLine = count(ctx, step.Line())
	}
	_, _ = f.WriteString(logLine + redact("+ "+strings.Join(cmd.Args, " ")) + "\n")
	cmd.Stdout, cmd.Stderr = f, f

	var s *statusLine
	if stderrTTY() {
		s = startStatus(label)
	}
	start := time.Now()
	err := run(cmd)
	if s != nil {
		s.stop()
		if took := time.Since(start); err == nil && took >= doneAfter {
			write(stderr, doneLine(label, int(took.Seconds())))
		}
	}
	return err
}

func run(cmd *exec.Cmd) error {
	commands.RLock()
	defer commands.RUnlock()
	return cmd.Run()
}
