package ui

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

func TestDuration(t *testing.T) {
	t.Parallel()
	for secs, want := range map[int]string{0: "0s", 5: "5s", 59: "59s", 60: "1m00s", 125: "2m05s"} {
		if got := duration(secs); got != want {
			t.Errorf("duration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestStatusText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		label   string
		secs    int
		logLine string
		cols    int
		want    string
	}{
		// The log line's last carriage-return segment, without tabs.
		{"running base.sh", 65, "Get:1 50%\rGet:2 https://example.com/x\t", 80, "running base.sh 1m05s  Get:2 https://example.com/x"},
		// As much as fits.
		{"running base.sh", 1, strings.Repeat("0", 100), 50, "running base.sh 1s  " + strings.Repeat("0", 23)},
		// No room: none of it.
		{"running base.sh", 1, "anything", 30, "running base.sh 1s"},
		{"starting x", 1, "", 80, "starting x 1s"},
		// No control characters from the log, C1 in UTF-8 included.
		{"x", 1, "a\033[2Jb\302\233c", 80, "x 1s  a[2Jbc"},
	} {
		if text, log := statusText(tc.label, tc.secs, tc.logLine, tc.cols); text+log != tc.want {
			got := text + log
			t.Errorf("statusText(%q, %d, %q, %d) = %q, want %q", tc.label, tc.secs, tc.logLine, tc.cols, got, tc.want)
		}
	}
}

func TestClearLine(t *testing.T) {
	t.Parallel()
	// After the terminal narrows, clearing covers the rows the last line now
	// wraps onto.
	for _, tc := range []struct {
		drawn, cols int
		want        string
	}{
		{100, 40, "\r\033[K\033[A\033[K\033[A\033[K"},
		{40, 40, "\r\033[K"},
		{100, 0, "\r\033[K\033[A\033[K"},
		{0, 80, "\r\033[K"},
	} {
		if got := clearLine(tc.drawn, tc.cols); got != tc.want {
			t.Errorf("clearLine(%d, %d) = %q, want %q", tc.drawn, tc.cols, got, tc.want)
		}
	}
}

func TestGlyphs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		env  map[string]string
		done string
	}{
		{map[string]string{"LANG": "en_US.UTF-8"}, "✓"},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, "ok"},
		{map[string]string{"LC_CTYPE": "en_US.utf8"}, "✓"},
		{map[string]string{}, "ok"},
	} {
		g := glyphs(func(k string) string { return tc.env[k] })
		if g[0] != tc.done || len(g) < 3 {
			t.Errorf("glyphs with %v = %q, want %q first, then frames", tc.env, g, tc.done)
		}
	}
	u, _, _ := testUI(t, "")
	u.getenv = env{"LC_ALL": "C"}.get
	if got, want := u.doneLine([]part{{"[1/2] starting x", ""}}, 75), "    ok [1/2] starting x (1m15s)\n"; got != want {
		t.Errorf("doneLine: got %q, want %q", got, want)
	}
}

func TestStatusLineInATerminal(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	u.stderrTTY = true
	u.termWidth = func() int { return 40 }
	u.getenv = env{"LC_ALL": "C"}.get
	if err := u.logged(t.Context(), msg.MsgUnknown("x"), "sh", "-c", "echo progress"); err != nil {
		t.Fatal(err)
	}
	got := errOut.String()
	// Drawn at least once, without wrapping, then cleared; quick, so it
	// leaves nothing behind.
	if !strings.HasPrefix(got, "\r\033[K\033[?7l    | msg_unknown name=x 0s") || !strings.HasSuffix(got, "\033[?7h\r\033[K") {
		t.Errorf("status line: %q", got)
	}
	for line := range strings.SplitSeq(got, "\033[?7l") {
		text, _, _ := strings.Cut(line, "\033[?7h")
		if len([]rune(text)) > 39 {
			t.Errorf("wider than the terminal: %q", text)
		}
	}

	// Status, around work that isn't one command.
	errOut.Reset()
	stop := u.Status(msg.MsgUnknown("y"))
	stop()
	stop()
	if got := errOut.String(); !strings.Contains(got, "msg_unknown name=y 0s") || !strings.HasSuffix(got, "\r\033[K") {
		t.Errorf("Status: %q", got)
	}
	// Not in a terminal, or verbose: none.
	errOut.Reset()
	u.stderrTTY = false
	u.Status(msg.MsgUnknown("y"))()
	u.stderrTTY = true
	u.SetFlags(Flags{Verbose: true})
	u.Status(msg.MsgUnknown("y"))()
	if errOut.Len() != 0 {
		t.Errorf("status line outside a terminal: %q", errOut)
	}
	// Nor for a logged command while verbose, whose lines are streamed.
	if err := u.logged(t.Context(), msg.MsgUnknown("z"), "true"); err != nil {
		t.Fatal(err)
	}
	if got := errOut.String(); strings.Contains(got, "msg_unknown") || !strings.Contains(got, "+ true\n") {
		t.Errorf("verbose logged command: %q", got)
	}
}

// While verbose, a logged failure names the run log, whose lines were
// streamed, instead of printing its end again.
func TestErrorLoggedVerbose(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	u.SetFlags(Flags{Verbose: true})
	_ = u.logged(t.Context(), msg.M{}, "sh", "-c", "echo failing; exit 1")
	u.ErrorLogged(msg.MsgUnknown("x"))
	want := "+ sh -c echo failing; exit 1\nfailing\nlog_details file=" + u.logPath() + "\ndevvm: msg_unknown name=x\n"
	if got := errOut.String(); got != want {
		t.Errorf("stderr %q, want %q", got, want)
	}
}

func TestTailLines(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/log"
	var b strings.Builder
	for i := range 30 {
		b.WriteString(strings.Repeat("x", i) + "\n")
	}
	writeFile(t, path, b.String())
	got := tailLines(path, 25)
	if lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n"); len(lines) != 25 || lines[24] != strings.Repeat("x", 29) {
		t.Errorf("tailLines: %q", got)
	}
	if got, want := lastLine(path), strings.Repeat("x", 29); got != want {
		t.Errorf("lastLine: %q, want %q", got, want)
	}
	if lastLine("") != "" || tailLines(path+".none", 25) != "" {
		t.Error("no file: want nothing")
	}
}
