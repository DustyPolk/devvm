package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

// logged runs argv through LoggedCmd.
func (u *UI) logged(ctx context.Context, step msg.M, argv ...string) error {
	return u.LoggedCmd(ctx, step, exec.CommandContext(ctx, argv[0], argv[1:]...)) //nolint:gosec // the tests' own commands
}

func writeFile(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil { //nolint:gosec // the test's own files
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // the test's own files
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoggedGoesToTheLog(t *testing.T) {
	t.Parallel()
	u, out, errOut := testUI(t, "")
	noisy := []string{"sh", "-c", `echo "lots of output"; echo "the error" >&2; exit "$0"`}
	if err := u.logged(t.Context(), msg.M{}, append(noisy, "0")...); err != nil {
		t.Fatal(err)
	}
	if out.Len()+errOut.Len() != 0 {
		t.Errorf("printed %q, %q; want nothing", out, errOut)
	}
	log := readFile(t, u.logPath())
	for _, want := range []string{"+ sh -c ", "\nlots of output\n", "\nthe error\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q: no %q", log, want)
		}
	}
	if logs, _ := u.runLogs(); filepath.Dir(u.logPath()) != logs {
		t.Errorf("log in %s, want it in %s", u.logPath(), logs)
	}
	// One log for the run.
	if err := u.logged(t.Context(), msg.M{}, append(noisy, "1")...); err == nil {
		t.Error("a failing command: no error")
	}
	if logs, _ := filepath.Glob(filepath.Join(filepath.Dir(u.logPath()), "*.log")); len(logs) != 1 {
		t.Errorf("logs: %q, want one", logs)
	}

	// LogNote says where it is, once.
	u.LogNote()
	u.LogNote()
	if got, want := out.String(), "    log_details file="+u.logPath()+"\n"; got != want {
		t.Errorf("LogNote: %q, want %q", got, want)
	}

	// DEVVM_VERBOSE prints it all instead.
	out.Reset()
	u.getenv = env{"DEVVM_VERBOSE": "1"}.get
	if err := u.logged(t.Context(), msg.M{}, append(noisy, "0")...); err != nil {
		t.Fatal(err)
	}
	u.LogNote()
	if out.String() != "lots of output\n" || errOut.String() != "the error\n" {
		t.Errorf("verbose: stdout %q, stderr %q", out, errOut)
	}
}

// The run logs are in devvm's data folder.
func TestRunLogsFolder(t *testing.T) {
	data := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", data)
	if dir, err := runLogs(); err != nil || dir != filepath.Join(data, "devvm", "logs", "runs") {
		t.Errorf("runLogs = %q, %v", dir, err)
	}
}

func TestDieLoggedShowsTheLogsEnd(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	_ = u.logged(t.Context(), msg.M{}, "sh", "-c", `printf 'guest\033]0;title\007 \302\233line\n'; exit 1`)
	if code := exitCode(func() { u.DieLogged(msg.MsgUnknown("x")) }); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	want := "--- log_end file=" + u.logPath() + "\n+ sh -c printf 'guest\\033]0;title\\007 \\302\\233line\\n'; exit 1\nguest]0;title line\n---\ndevvm: msg_unknown name=x\n"
	if got := errOut.String(); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// Without a log, just the message.
	u, _, errOut = testUI(t, "")
	exitCode(func() { u.DieLogged(msg.MsgUnknown("x")) })
	if got, want := errOut.String(), "devvm: msg_unknown name=x\n"; got != want {
		t.Errorf("no log: got %q, want %q", got, want)
	}
}

func TestProxyKeysStayOutOfTheLog(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI(t, "")
	key := strings.Repeat("0123456789abcdef", 2)
	if err := u.logged(t.Context(), msg.M{}, "true", "--set", `.env.HTTP_PROXY="http://devvm:`+key+`@host.lima.internal:3128"`); err != nil {
		t.Fatal(err)
	}
	log := readFile(t, u.logPath())
	if strings.Contains(log, key) || !strings.Contains(log, "http://devvm:<key>@host") {
		t.Errorf("log: %q", log)
	}
	// Owner-only.
	for path, want := range map[string]os.FileMode{
		filepath.Dir(filepath.Dir(u.logPath())): 0o700,
		filepath.Dir(u.logPath()):               0o700,
		u.logPath():                             0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, want %v (%v)", path, fi.Mode().Perm(), want, err)
		}
	}
}

func TestStepCounts(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI(t, "")
	ctx := u.WithSteps(t.Context(), 2)
	for _, s := range []string{"a", "b", "c"} {
		if err := u.logged(ctx, msg.MsgUnknown(s), "true"); err != nil {
			t.Fatal(err)
		}
	}
	// A phase inside another keeps its own count; an unlabelled step and one
	// outside a phase aren't counted.
	if err := u.logged(u.WithSteps(ctx, 1), msg.MsgUnknown("inner"), "true"); err != nil {
		t.Fatal(err)
	}
	if err := u.logged(ctx, msg.M{}, "true"); err != nil {
		t.Fatal(err)
	}
	if err := u.logged(t.Context(), msg.MsgUnknown("outside"), "true"); err != nil {
		t.Fatal(err)
	}
	want := "# [1/2] msg_unknown name=a\n+ true\n# [2/2] msg_unknown name=b\n+ true\n# [3/2] msg_unknown name=c\n+ true\n" +
		"# [1/1] msg_unknown name=inner\n+ true\n+ true\n+ true\n"
	if got := readFile(t, u.logPath()); got != want {
		t.Errorf("log:\n%s\nwant:\n%s", got, want)
	}

	// The status line's label: numbered while in the count.
	ctx = u.WithSteps(context.Background(), 1)
	for _, want := range []string{"[1/1] x", "x"} {
		if got, _ := count(ctx, "x"); got != want {
			t.Errorf("label %q, want %q", got, want)
		}
	}
}

func TestKeepsTheLast20Logs(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI(t, "")
	runs, _ := u.runLogs()
	logs := filepath.Dir(runs)
	if err := os.MkdirAll(runs, 0o700); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for i := range 25 {
		for _, f := range []string{
			filepath.Join(runs, fmt.Sprintf("old-%d.log", i)),
			filepath.Join(logs, fmt.Sprintf("devvm-p%d-abc123.proxy.log", i)),
		} {
			writeFile(t, f, "")
			mod := old.Add(time.Duration(i) * time.Minute)
			if err := os.Chtimes(f, mod, mod); err != nil {
				t.Fatal(err)
			}
		}
	}
	u.LogNote()
	kept, _ := filepath.Glob(filepath.Join(runs, "*.log"))
	proxy, _ := filepath.Glob(filepath.Join(logs, "*.proxy.log"))
	if len(kept) != 20 || len(proxy) != 25 {
		t.Errorf("%d run logs and %d proxy logs, want 20 and 25", len(kept), len(proxy))
	}
	for _, f := range kept {
		if f == filepath.Join(runs, "old-5.log") {
			t.Errorf("kept an older log: %s", f)
		}
	}
	if _, err := os.Stat(u.logPath()); err != nil {
		t.Errorf("this run's log: %v", err)
	}
}

func TestRunLogFailures(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	u.runLogs = func() (string, error) { return "", msg.HomeNotAbsolute() }
	if code := exitCode(u.LogNote); code != 1 || errOut.String() != "devvm: home_not_absolute\n" {
		t.Errorf("exit code %d, stderr %q", code, errOut)
	}
	u, _, errOut = testUI(t, "")
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "devvm"), "a file, not a folder")
	runs := filepath.Join(data, "devvm", "logs", "runs")
	u.runLogs = func() (string, error) { return runs, nil }
	if code := exitCode(u.LogNote); code != 1 || !strings.HasPrefix(errOut.String(), "devvm: run_log_failed dir="+runs+" reason=") {
		t.Errorf("exit code %d, stderr %q", code, errOut)
	}
}
