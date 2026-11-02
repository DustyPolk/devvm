package msg

import (
	"strings"
	"testing"
)

func TestPrintable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, printable, oneLine string }{
		{"guest\033]0;title\007 \302\233line\n", "guest]0;title line\n", "guest]0;title line"},
		{"tab\tand\nnewline", "tab\tand\nnewline", "tabandnewline"},
		{"del\177 nul\000 c1\302\237 é ü", "del nul c1 é ü", "del nul c1 é ü"},
		// A lone C1 byte goes; other invalid UTF-8 stays.
		{"a\x9bb\xffc\302", "ab\xffc\302", "ab\xffc\302"},
		{"cr\r\u0085x", "crx", "crx"},
		{"/w/My App", "/w/My App", "/w/My App"},
		{"/w/café", "/w/café", "/w/café"},
		{"../x\x1b[2J", "../x[2J", "../x[2J"},
	} {
		if got := Printable(tc.in); got != tc.printable {
			t.Errorf("Printable(%q) = %q, want %q", tc.in, got, tc.printable)
		}
		if got := OneLine(tc.in); got != tc.oneLine {
			t.Errorf("OneLine(%q) = %q, want %q", tc.in, got, tc.oneLine)
		}
	}
}

// Every value of a message renders on one line, without control
// characters, whoever made it: in its text, its segments and its names.
func TestValuesArePrintable(t *testing.T) {
	hostile := "vm\033[2J\033]0;title\007\ndevvm: \302\233x"
	m := PortsNoSSH(hostile)
	t.Setenv("DEVVM_MESSAGES", "")
	text := m.Text()
	segs, _ := m.Segments()
	var joined strings.Builder
	for _, s := range segs {
		joined.WriteString(s.Text)
	}
	t.Setenv("DEVVM_MESSAGES", "names")
	for what, s := range map[string]string{"text": text, "segments": joined.String(), "names": m.Names()} {
		if s != Printable(s) || strings.Contains(s, "\n") || !strings.Contains(s, "vm[2J]0;titledevvm: x") {
			t.Errorf("%s: %q", what, s)
		}
	}
}
