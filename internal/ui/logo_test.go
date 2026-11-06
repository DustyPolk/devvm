package ui

import "testing"

func TestLogo(t *testing.T) {
	const plain = "╭──────────╮\n│ ❯ devvm ╭┤\n╰─────────██\n"
	const colored = "╭──────────╮\n│ \033[32m❯\033[0m devvm ╭┤\n╰─────────██\n"
	for _, tc := range []struct {
		name                     string
		terminal, pipe, noInput  bool
		dataOnly, names, colorOn bool
		want                     string
	}{
		{name: "terminal, colored", terminal: true, colorOn: true, want: colored},
		{name: "terminal, NO_COLOR", terminal: true, want: plain},
		{name: "PickOnPipe", pipe: true, want: plain},
		{name: "no terminal"},
		{name: "names mode", terminal: true, names: true},
		{name: "no input", terminal: true, noInput: true},
		{name: "json", terminal: true, dataOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, out, _ := testUI(t, "")
			u.stdoutTTY = tc.terminal
			u.pickOnPipe.Store(tc.pipe)
			u.noInput.Store(tc.noInput)
			u.dataOnly.Store(tc.dataOnly)
			u.outStyle = style{on: tc.colorOn, names: tc.names}
			u.Logo()
			u.Logo() // once a run
			if got := out.String(); got != tc.want {
				t.Errorf("Logo printed %q, want %q", got, tc.want)
			}
		})
	}
}

// NO_COLOR turns the logo's color off in a terminal, as colorOn has it.
func TestLogoNoColor(t *testing.T) {
	if s := newStyle(env{"NO_COLOR": "1"}.get, true); s.logo() != logoText {
		t.Errorf("NO_COLOR's logo is %q, want it plain", s.logo())
	}
	if s := newStyle(env{"DEVVM_MESSAGES": "names"}.get, true); !s.names {
		t.Error("DEVVM_MESSAGES=names isn't names mode")
	}
}
