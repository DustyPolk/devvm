package ui

import (
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

func TestDuration(t *testing.T) {
	for secs, want := range map[int]string{0: "0s", 5: "5s", 59: "59s", 60: "1m00s", 125: "2m05s"} {
		if got := duration(secs); got != want {
			t.Errorf("duration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestStatusText(t *testing.T) {
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
		if got := statusText(tc.label, tc.secs, tc.logLine, tc.cols); got != tc.want {
			t.Errorf("statusText(%q, %d, %q, %d) = %q, want %q", tc.label, tc.secs, tc.logLine, tc.cols, got, tc.want)
		}
	}
}

func TestClearLine(t *testing.T) {
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

func TestPrintable(t *testing.T) {
	for in, want := range map[string]string{
		"guest\033]0;title\007 \302\233line\n": "guest]0;title line\n",
		"tab\tand\nnewline":                    "tab\tand\nnewline",
		"del\177 nul\000 c1\302\237 é ü":       "del nul c1 é ü",
		"\302":                                 "\302",
	} {
		if got := printable(in); got != want {
			t.Errorf("printable(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlyphs(t *testing.T) {
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
	t.Setenv("LC_ALL", "C")
	if got, want := doneLine("[1/2] starting x", 75), "    ok [1/2] starting x (1m15s)\n"; got != want {
		t.Errorf("doneLine: got %q, want %q", got, want)
	}
}

func TestStatusLineInATerminal(t *testing.T) {
	_, errOut := sandbox(t, "")
	stderrTTY = func() bool { return true }
	termWidth = func() int { return 40 }
	t.Setenv("LC_ALL", "C")
	if err := Logged(t.Context(), msg.MsgUnknown("x"), "sh", "-c", "echo progress"); err != nil {
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
	stop := Status(msg.MsgUnknown("y"))
	stop()
	stop()
	if got := errOut.String(); !strings.Contains(got, "msg_unknown name=y 0s") || !strings.HasSuffix(got, "\r\033[K") {
		t.Errorf("Status: %q", got)
	}
	// Not in a terminal, or verbose: none.
	errOut.Reset()
	stderrTTY = func() bool { return false }
	Status(msg.MsgUnknown("y"))()
	stderrTTY = func() bool { return true }
	t.Setenv("DEVVM_VERBOSE", "1")
	Status(msg.MsgUnknown("y"))()
	if errOut.Len() != 0 {
		t.Errorf("status line outside a terminal: %q", errOut)
	}
}

func TestTailLines(t *testing.T) {
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
