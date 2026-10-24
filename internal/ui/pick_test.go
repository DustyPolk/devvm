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

// pickTerminal makes Pick see terminals, with raw mode a no-op that
// counts its restores.
func pickTerminal(t *testing.T) *int {
	t.Helper()
	oldIn, oldOut, oldRaw := stdinTTY, stdoutTTY, rawInput
	restored := 0
	stdinTTY = func() bool { return true }
	stdoutTTY = func() bool { return true }
	rawInput = func() (func(), error) { return func() { restored++ }, nil }
	t.Cleanup(func() { stdinTTY, stdoutTTY, rawInput = oldIn, oldOut, oldRaw })
	return &restored
}

func TestPickKeys(t *testing.T) {
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
			out, _ := sandbox(t, tc.keys)
			restored := pickTerminal(t)
			in := choices()
			got := Pick(msg.MsgUnknown("x"), in)
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

func TestPickWithoutTerminal(t *testing.T) {
	out, _ := sandbox(t, " \r")
	got := Pick(msg.MsgUnknown("x"), choices())
	if !slices.Equal(onOf(got), []string{"a", "b"}) || out.Len() != 0 {
		t.Errorf("chosen %q, printed %q: want the choices as given, nothing printed", onOf(got), out)
	}
}

func TestPickCtrlC(t *testing.T) {
	sandbox(t, "j\x03 \r")
	restored := pickTerminal(t)
	if code := exitCode(func() { Pick(msg.MsgUnknown("x"), choices()) }); code != InterruptedCode {
		t.Errorf("exit code %d, want %d", code, InterruptedCode)
	}
	if !Interrupted() || *restored != 1 {
		t.Errorf("interrupted %v, raw mode undone %d times", Interrupted(), *restored)
	}
}
