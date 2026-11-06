package ui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/devvmhq/devvm/internal/msg"
)

// sgr matches the SGR codes devvm prints.
var sgr = regexp.MustCompile("\033\\[[0-9;]*m")

func plain(s string) string { return sgr.ReplaceAllString(s, "") }

func TestColorOn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		env      env
		terminal bool
		want     bool
	}{
		{env{}, true, true},
		{env{}, false, false},
		{env{"TERM": "xterm-256color"}, true, true},
		{env{"TERM": "dumb"}, true, false},
		{env{"NO_COLOR": "1"}, true, false},
		{env{"NO_COLOR": ""}, true, true}, // set but empty: not set
		{env{"FORCE_COLOR": "1"}, false, true},
		{env{"CLICOLOR_FORCE": "1"}, false, true},
		{env{"FORCE_COLOR": "1", "TERM": "dumb"}, false, true},
		{env{"FORCE_COLOR": "0"}, false, false},
		{env{"CLICOLOR_FORCE": "0"}, true, true},
		{env{"FORCE_COLOR": "1", "NO_COLOR": "1"}, true, false},
		{env{"DEVVM_MESSAGES": "names"}, true, false},
		{env{"DEVVM_MESSAGES": "names", "FORCE_COLOR": "1"}, true, false},
	} {
		if got := colorOn(tc.env.get, tc.terminal); got != tc.want {
			t.Errorf("colorOn(%v, terminal %v) = %v, want %v", tc.env, tc.terminal, got, tc.want)
		}
	}
}

func TestMessagesColored(t *testing.T) {
	t.Parallel()
	linked := msg.StopFailed("v")
	plainText, link := linked.Parts()
	if link == "" || strings.Contains(plainText, link) {
		t.Fatalf("stop_failed: want a doc link apart from the text, got %q, %q", plainText, link)
	}
	// Its value bold, the rest as it is.
	var text string
	segs, _ := linked.Segments()
	for _, s := range segs {
		if s.Value {
			text += "\033[1m" + s.Text + "\033[0m"
		} else {
			text += s.Text
		}
	}
	if !strings.Contains(text, "\033[1mv\033[0m") || plain(text) != plainText {
		t.Fatalf("stop_failed's value: %q", text)
	}
	for _, tc := range []struct {
		name          string
		print         func(*UI, msg.M)
		stdout, stder string
	}{
		{"say", (*UI).Say, text + "\n\033[4;34m" + link + "\033[0m\n", ""},
		{"info", (*UI).Info, "\033[1m==>\033[0m " + text + "\n\033[4;34m" + link + "\033[0m\n", ""},
		{"note", (*UI).Note, "", text + "\n\033[4;34m" + link + "\033[0m\n"},
		{"warn", (*UI).Warn, "", "\033[1;33mdevvm:\033[0m " + text + "\n\033[1;33mdevvm:\033[0m \033[4;34m" + link + "\033[0m\n"},
		{"error", (*UI).Error, "", "\033[1;31mdevvm:\033[0m " + text + "\n\033[1;31mdevvm:\033[0m \033[4;34m" + link + "\033[0m\n"},
	} {
		u, out, errOut := testUI(t, "")
		u.outStyle, u.errStyle = style{on: true}, style{on: true}
		tc.print(u, linked)
		if out.String() != tc.stdout || errOut.String() != tc.stder {
			t.Errorf("%s colored: stdout %q, stderr %q; want %q, %q", tc.name, out, errOut, tc.stdout, tc.stder)
		}
		// Plain, it's the same text without the codes.
		u, out, errOut = testUI(t, "")
		u.outStyle, u.errStyle = style{}, style{}
		tc.print(u, linked)
		if got := out.String() + errOut.String(); got != plain(tc.stdout+tc.stder) {
			t.Errorf("%s plain: %q", tc.name, got)
		}
	}
	if plainText+"\n"+link != linked.String() {
		t.Errorf("String: %q", linked.String())
	}
}

// DEVVM_MESSAGES=names gives a stream's style names mode, without color: a
// message shows as its names, one segment, no link, wherever the UI shows
// it, and a terminal gets no codes, so e2e's checks of one read the names
// as they are.
func TestNamesMode(t *testing.T) {
	t.Parallel()
	names := env{"DEVVM_MESSAGES": "names", "FORCE_COLOR": "1"}.get
	st := newStyle(names, true)
	if st != (style{names: true}) || newStyle(env{}.get, false) != (style{}) {
		t.Fatalf("newStyle: %+v", st)
	}
	m := msg.StopFailed("v")
	if segs, link := st.segments(m); len(segs) != 1 || segs[0] != (msg.Segment{Text: m.Names()}) || link != "" {
		t.Errorf("segments: %+v, %q", segs, link)
	}
	if st.message(m) != m.Names() || st.line(m) != m.Names() || st.line(msg.M{}) != "" {
		t.Errorf("message %q, line %q", st.message(m), st.line(m))
	}
	if text := (style{}).line(m); text != m.Text() {
		t.Errorf("line, not names mode: %q", text)
	}
	u, out, errOut := testUI(t, "")
	u.Say(m)
	u.Warn(m)
	if out.String() != m.Names()+"\n" || errOut.String() != "devvm: "+m.Names()+"\n" || u.Text(m) != m.Names() || u.Table().Text(m) != m.Names() {
		t.Errorf("stdout %q, stderr %q, Text %q", out, errOut, u.Text(m))
	}
	// The messages themselves don't change: String is still the text.
	if m.String() == m.Names() {
		t.Errorf("String in names mode: %q", m.String())
	}
}

func TestErrorLoggedColored(t *testing.T) {
	u, _, errOut := testUI(t, "")
	u.errStyle = style{on: true, names: true}
	if err := u.logged(t.Context(), msg.M{}, "sh", "-c", "printf 'one\\n\\033[31mtwo\\n'"); err != nil {
		t.Fatal(err)
	}
	u.ErrorLogged(msg.MsgUnknown("x"))
	got := errOut.String()
	// Each line of the log's end dim, the log's own codes dropped; then the
	// message, its label red.
	head := "\033[2m--- " + msg.LogEnd(u.logPath()).Names() + "\033[0m\n"
	want := "\033[2mone\033[0m\n\033[2m[31mtwo\033[0m\n\033[2m---\033[0m\n" +
		"\033[1;31mdevvm:\033[0m msg_unknown name=x\n"
	if !strings.HasPrefix(got, head) || !strings.HasSuffix(got, want) {
		t.Errorf("ErrorLogged colored:\n%q\nwant it to end\n%q", got, want)
	}
}

func TestColoredStatusLineFits(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	u.stderrTTY, u.errStyle = true, style{on: true, names: true}
	u.termWidth = func() int { return 40 }
	u.getenv = env{"LC_ALL": "C"}.get
	if err := u.logged(t.Context(), msg.MsgUnknown("x"), "sh", "-c", "echo "+strings.Repeat("progress", 10)+"; sleep 0.5"); err != nil {
		t.Fatal(err)
	}
	got := errOut.String()
	if !strings.Contains(got, "\033[2m  progress") {
		t.Errorf("status line: want the log's line dim: %q", got)
	}
	// The plain text fits in the width, whatever the codes add.
	for line := range strings.SplitSeq(got, "\033[?7l") {
		text, _, _ := strings.Cut(line, "\033[?7h")
		if n := utf8.RuneCountInString(plain(text)); n > 39 {
			t.Errorf("wider than the terminal: %d runes: %q", n, text)
		}
	}
	// What fit counts the plain text, so clearing covers the line drawn.
	line, n := u.errStyle.fit([]part{{strings.Repeat("a", 30), ""}, {strings.Repeat("b", 30), sgrDim}}, 39)
	if n != 39 || plain(line) != strings.Repeat("a", 30)+strings.Repeat("b", 9) || clearLine(n, 40) != "\r\033[K" {
		t.Errorf("fit: %q, %d", line, n)
	}
}

// A status line's values are bold, and it's cut on its plain text, a
// value included, with the value's code closed.
func TestStatusLineValuesBold(t *testing.T) {
	t.Parallel()
	u, _, errOut := testUI(t, "")
	u.stderrTTY, u.errStyle = true, style{on: true}
	u.getenv = env{"LC_ALL": "C"}.get
	m := msg.StopFailed("a-long-vm-name")
	for _, cols := range []int{80, 12} {
		errOut.Reset()
		u.termWidth = func() int { return cols }
		u.Status(m)()
		got := errOut.String()
		text, _, _ := strings.Cut(strings.TrimPrefix(got, "\r\033[K\033[?7l"), "\033[?7h")
		if n := utf8.RuneCountInString(plain(text)); n > cols-1 {
			t.Errorf("%d columns: %d runes: %q", cols, n, text)
		}
		want := "\033[1ma-long-vm-name\033[0m"
		if cols == 12 {
			want = "\033[1ma-lon\033[0m" // 11 columns: "    | " and 5 runes of the value
		}
		if !strings.Contains(text, want) {
			t.Errorf("%d columns: no %q in %q", cols, want, text)
		}
	}
	// Plain, it's the same text.
	u.errStyle = style{}
	u.termWidth = func() int { return 80 }
	errOut.Reset()
	u.Status(m)()
	if !strings.Contains(errOut.String(), "    | "+m.Text()+" 0s") {
		t.Errorf("plain: %q", errOut.String())
	}
}

func TestDoneMarkGreen(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI(t, "")
	u.getenv = env{"LC_ALL": "C"}.get
	u.errStyle = style{on: true}
	if got, want := u.doneLine([]part{{"starting ", ""}, {"x", sgrBold}}, 5), "    \033[32mok\033[0m starting \033[1mx\033[0m (5s)\n"; got != want {
		t.Errorf("doneLine: got %q, want %q", got, want)
	}
}
