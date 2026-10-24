package ui

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"

	"github.com/w3cj/devvm/internal/msg"
)

// Choice is one of the items Pick shows: a name, what it is, and whether
// it's chosen.
type Choice struct {
	Name, Desc string
	On         bool
}

// rawInput puts the terminal devvm reads from into raw mode, returning
// what undoes it. Swapped by the tests.
var rawInput = func() (restore func(), err error) {
	old, err := term.MakeRaw(fd(os.Stdin))
	if err != nil {
		return nil, err
	}
	return func() { _ = term.Restore(fd(os.Stdin), old) }, nil
}

// Pick shows the choices under help and lets the user toggle them: up and
// down (or k and j) move, space toggles, enter confirms. It returns them
// as chosen. Without a terminal to read from and print to, it returns
// them as given. Ctrl+C ends the run as interrupted.
func Pick(help msg.M, choices []Choice) []Choice {
	out := slices.Clone(choices)
	if len(out) == 0 || !stdinTTY() || !stdoutTTY() {
		return out
	}
	restore, err := rawInput()
	if err != nil {
		return out
	}
	// In raw mode a line ends with \r\n; the cursor is hidden meanwhile.
	write(stdout, strings.ReplaceAll(help.String(), "\n", "\r\n")+"\r\n\033[?25l")
	ok := pickLoop(stdin, out)
	write(stdout, "\033[?25h")
	restore()
	if !ok {
		write(stderr, "\n")
		interrupted.Store(true)
		exit(InterruptedCode)
	}
	return out
}

// pickLoop draws the choices and reads keys until enter (or the end of
// the input), toggling choices in place. It's false on Ctrl+C.
func pickLoop(in *bufio.Reader, choices []Choice) bool {
	cur, n := 0, len(choices)
	for {
		var b strings.Builder
		for i, c := range choices {
			mark, ptr := " ", " "
			if c.On {
				mark = "x"
			}
			if i == cur {
				ptr = ">"
			}
			fmt.Fprintf(&b, "\033[2K%s [%s] %-10s %s\r\n", ptr, mark, printable(c.Name), printable(c.Desc))
		}
		write(stdout, b.String())
		key, err := in.ReadByte()
		if err != nil {
			return true
		}
		switch key {
		case 0x03: // Ctrl+C: raw mode turns it into input
			return false
		case '\r', '\n':
			return true
		case ' ':
			choices[cur].On = !choices[cur].On
		case 'k':
			cur = (cur + n - 1) % n
		case 'j':
			cur = (cur + 1) % n
		case 0x1b: // an arrow: ESC [ A or ESC [ B
			if seq, err := in.Peek(2); err == nil && seq[0] == '[' {
				switch seq[1] {
				case 'A':
					cur = (cur + n - 1) % n
				case 'B':
					cur = (cur + 1) % n
				}
				_, _ = in.Discard(2)
			}
		}
		write(stdout, fmt.Sprintf("\033[%dA", n))
	}
}
