package msg

import (
	"strings"
	"unicode/utf8"
)

// Printable returns s without the characters Safe refuses, but newline
// and tab, and without a lone C1 byte: nothing in it moves the cursor,
// rewrites, reorders or hides what a terminal shows. It's for text a VM
// or a build made (its output, a path, a host), and every message value
// goes through OneLine, its one-line form, when the message renders.
// Other bytes, invalid UTF-8 among them, stay as they are.
func Printable(s string) string { return clean(s, true) }

// OneLine is Printable without newlines and tabs too: text for one line,
// such as a message value, a status line or a log entry.
func OneLine(s string) string { return clean(s, false) }

func clean(s string, lines bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			if c := s[i]; c < 0x80 || c > 0x9f {
				b.WriteByte(c)
			}
		case r == '\n' || r == '\t':
			if lines {
				b.WriteRune(r)
			}
		case !Safe(r):
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
