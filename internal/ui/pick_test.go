package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/msg"
)

func choices() []Choice {
	return []Choice{{"a", "first", true}, {"b", "second", true}, {"c", "third\x1b[2J", false}}
}

func onOf(cs []Choice) []string {
	var on []string
	for _, c := range cs {
		if c.On {
			on = append(on, c.Name)
		}
	}
	return on
}

// pickTerminal makes u's Pick see terminals, with raw mode a no-op that
// counts its restores.
func pickTerminal(u *UI) *int {
	restored := 0
	u.stdinTTY, u.stdoutTTY = true, true
	u.rawInput = func() (func(), error) { return func() { restored++ }, nil }
	return &restored
}

func TestPickKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, keys string
		want       []string
	}{
		{"enter keeps them", "\r", []string{"a", "b"}},
		{"space toggles the first", " \r", []string{"b"}},
		{"j and space", "j \r", []string{"a"}},
		{"down arrow twice, space", "\x1b[B\x1b[B \r", []string{"a", "b", "c"}},
		{"up wraps to the last", "\x1b[A \r", []string{"a", "b", "c"}},
		{"k wraps, j back", "kj \r", []string{"b"}},
		{"other keys do nothing", "xq\x1b[C\r", []string{"a", "b"}},
		{"the end of input confirms", " ", []string{"b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, out, _ := testUI(t, tc.keys)
			restored := pickTerminal(u)
			in := choices()
			got := u.Pick(msg.MsgUnknown("x"), in)
			if !slices.Equal(onOf(got), tc.want) {
				t.Errorf("chosen %q, want %q", onOf(got), tc.want)
			}
			if !slices.Equal(onOf(in), []string{"a", "b"}) {
				t.Error("Pick changed its argument")
			}
			if *restored != 1 {
				t.Errorf("raw mode undone %d times, want 1", *restored)
			}
			s := out.String()
			if !strings.HasPrefix(s, "msg_unknown name=x\r\n\033[?25l") || !strings.HasSuffix(s, "\033[?25h") {
				t.Errorf("output %q: no help first, or the cursor not shown again", s)
			}
			if !strings.Contains(s, "> [x] a          first\r\n") || strings.Contains(s, "\x1b[2J") {
				t.Errorf("output %q: rows not drawn, or a control sequence from a description", s)
			}
		})
	}
}

func TestPickOneKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, keys string
		start      int // the choice on; -1 for none
		want       int
	}{
		{"enter keeps the current", "\r", 1, 1},
		{"none on starts at the first", "\r", -1, 0},
		{"j moves down", "j\r", 0, 1},
		{"down arrow twice", "\x1b[B\x1b[B\r", 0, 2},
		{"up wraps to the last", "\x1b[A\r", 0, 2},
		{"k from the current", "k\r", 2, 1},
		{"space and other keys do nothing", " xq\x1b[C\r", 1, 1},
		{"the end of input confirms", "j", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			u, out, _ := testUI(t, tc.keys)
			restored := pickTerminal(u)
			in := []Choice{{"a", "first", false}, {"b", "second", false}, {"c", "third", false}}
			if tc.start >= 0 {
				in[tc.start].On = true
			}
			if got := u.PickOne(msg.MsgUnknown("x"), in); got != tc.want {
				t.Errorf("chosen %d, want %d", got, tc.want)
			}
			if *restored != 1 {
				t.Errorf("raw mode undone %d times, want 1", *restored)
			}
			s := out.String()
			if !strings.HasPrefix(s, "msg_unknown name=x\r\n\033[?25l") || !strings.HasSuffix(s, "\033[?25h") {
				t.Errorf("output %q: no help first, or the cursor not shown again", s)
			}
			first := max(tc.start, 0)
			if row := "> (*) " + in[first].Name; !strings.Contains(s, row) {
				t.Errorf("output %q: no row %q", s, row)
			}
		})
	}
}

func TestPickOneWithoutTerminal(t *testing.T) {
	t.Parallel()
	u, out, _ := testUI(t, "j\r")
	if got := u.PickOne(msg.MsgUnknown("x"), choices()); got != 0 || out.Len() != 0 {
		t.Errorf("chosen %d, printed %q: want the first on, nothing printed", got, out)
	}
}

func TestPickWithoutTerminal(t *testing.T) {
	t.Parallel()
	u, out, _ := testUI(t, " \r")
	got := u.Pick(msg.MsgUnknown("x"), choices())
	if !slices.Equal(onOf(got), []string{"a", "b"}) || out.Len() != 0 {
		t.Errorf("chosen %q, printed %q: want the choices as given, nothing printed", onOf(got), out)
	}
}

func TestPickCtrlC(t *testing.T) {
	t.Parallel()
	u, _, _ := testUI(t, "j\x03 \r")
	restored := pickTerminal(u)
	if code := exitCode(func() { u.Pick(msg.MsgUnknown("x"), choices()) }); code != InterruptedCode {
		t.Errorf("exit code %d, want %d", code, InterruptedCode)
	}
	if !u.Interrupted() || *restored != 1 {
		t.Errorf("interrupted %v, raw mode undone %d times", u.Interrupted(), *restored)
	}
}
