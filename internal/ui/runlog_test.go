package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

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
	out, errOut := sandbox(t, "")
	noisy := []string{"sh", "-c", `echo "lots of output"; echo "the error" >&2; exit "$0"`}
	if err := Logged(t.Context(), msg.M{}, append(noisy, "0")...); err != nil {
		t.Fatal(err)
	}
	if out.Len()+errOut.Len() != 0 {
		t.Errorf("printed %q, %q; want nothing", out, errOut)
	}
	log := readFile(t, logPath())
	for _, want := range []string{"+ sh -c ", "\nlots of output\n", "\nthe error\n"} {
		if !strings.Contains(log, want) {
			t.Errorf("log %q: no %q", log, want)
		}
	}
	if !strings.HasPrefix(logPath(), filepath.Join(os.Getenv("XDG_DATA_HOME"), "devvm", "logs", "runs")+"/") {
		t.Errorf("log in %s", logPath())
	}
	// One log for the run.
	if err := Logged(t.Context(), msg.M{}, append(noisy, "1")...); err == nil {
		t.Error("a failing command: no error")
	}
	if logs, _ := filepath.Glob(filepath.Join(filepath.Dir(logPath()), "*.log")); len(logs) != 1 {
		t.Errorf("logs: %q, want one", logs)
	}

	// LogNote says where it is, once.
	LogNote()
	LogNote()
	if got, want := out.String(), "    log_details file="+logPath()+"\n"; got != want {
		t.Errorf("LogNote: %q, want %q", got, want)
	}

	// DEVVM_VERBOSE prints it all instead.
	out.Reset()
	t.Setenv("DEVVM_VERBOSE", "1")
	if err := Logged(t.Context(), msg.M{}, append(noisy, "0")...); err != nil {
		t.Fatal(err)
	}
	LogNote()
	if out.String() != "lots of output\n" || errOut.String() != "the error\n" {
		t.Errorf("verbose: stdout %q, stderr %q", out, errOut)
	}
}

func TestDieLoggedShowsTheLogsEnd(t *testing.T) {
	_, errOut := sandbox(t, "")
	_ = Logged(t.Context(), msg.M{}, "sh", "-c", `printf 'guest\033]0;title\007 \302\233line\n'; exit 1`)
	if code := exitCode(func() { DieLogged(msg.MsgUnknown("x")) }); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	want := "--- log_end file=" + logPath() + "\n+ sh -c printf 'guest\\033]0;title\\007 \\302\\233line\\n'; exit 1\nguest]0;title line\n---\ndevvm: msg_unknown name=x\n"
	if got := errOut.String(); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	// Without a log, just the message.
	_, errOut = sandbox(t, "")
	exitCode(func() { DieLogged(msg.MsgUnknown("x")) })
	if got, want := errOut.String(), "devvm: msg_unknown name=x\n"; got != want {
		t.Errorf("no log: got %q, want %q", got, want)
	}
}

func TestProxyKeysStayOutOfTheLog(t *testing.T) {
	sandbox(t, "")
	key := strings.Repeat("0123456789abcdef", 2)
	if err := Logged(t.Context(), msg.M{}, "true", "--set", `.env.HTTP_PROXY="http://devvm:`+key+`@host.lima.internal:3128"`); err != nil {
		t.Fatal(err)
	}
	log := readFile(t, logPath())
	if strings.Contains(log, key) || !strings.Contains(log, "http://devvm:<key>@host") {
		t.Errorf("log: %q", log)
	}
	// Owner-only.
	for path, want := range map[string]os.FileMode{
		filepath.Dir(filepath.Dir(logPath())): 0o700,
		filepath.Dir(logPath()):               0o700,
		logPath():                             0o600,
	} {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: %v, want %v (%v)", path, fi.Mode().Perm(), want, err)
		}
	}
}

func TestStepCounts(t *testing.T) {
	sandbox(t, "")
	ctx := WithSteps(t.Context(), 2)
	for _, s := range []string{"a", "b", "c"} {
		if err := Logged(ctx, msg.MsgUnknown(s), "true"); err != nil {
			t.Fatal(err)
		}
	}
	// A phase inside another keeps its own count; an unlabelled step and one
	// outside a phase aren't counted.
	if err := Logged(WithSteps(ctx, 1), msg.MsgUnknown("inner"), "true"); err != nil {
		t.Fatal(err)
	}
	if err := Logged(ctx, msg.M{}, "true"); err != nil {
		t.Fatal(err)
	}
	if err := Logged(t.Context(), msg.MsgUnknown("outside"), "true"); err != nil {
		t.Fatal(err)
	}
	want := "# [1/2] msg_unknown name=a\n+ true\n# [2/2] msg_unknown name=b\n+ true\n# [3/2] msg_unknown name=c\n+ true\n" +
		"# [1/1] msg_unknown name=inner\n+ true\n+ true\n+ true\n"
	if got := readFile(t, logPath()); got != want {
		t.Errorf("log:\n%s\nwant:\n%s", got, want)
	}

	// The status line's label: numbered while in the count.
	ctx = WithSteps(context.Background(), 1)
	for _, want := range []string{"[1/1] x", "x"} {
		if got, _ := count(ctx, "x"); got != want {
			t.Errorf("label %q, want %q", got, want)
		}
	}
}

func TestKeepsTheLast20Logs(t *testing.T) {
	sandbox(t, "")
	logs := filepath.Join(os.Getenv("XDG_DATA_HOME"), "devvm", "logs")
	runs := filepath.Join(logs, "runs")
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
	LogNote()
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
	if _, err := os.Stat(logPath()); err != nil {
		t.Errorf("this run's log: %v", err)
	}
}

func TestRunLogFailures(t *testing.T) {
	_, errOut := sandbox(t, "")
	t.Setenv("HOME", "relative")
	if code := exitCode(LogNote); code != 1 || errOut.String() != "devvm: home_not_absolute\n" {
		t.Errorf("exit code %d, stderr %q", code, errOut)
	}
	_, errOut = sandbox(t, "")
	data := os.Getenv("XDG_DATA_HOME")
	writeFile(t, filepath.Join(data, "devvm"), "a file, not a folder")
	if code := exitCode(LogNote); code != 1 || !strings.HasPrefix(errOut.String(), "devvm: run_log_failed dir="+filepath.Join(data, "devvm", "logs", "runs")+" reason=") {
		t.Errorf("exit code %d, stderr %q", code, errOut)
	}
}
