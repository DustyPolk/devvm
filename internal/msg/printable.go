package msg

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Printable returns s without the control characters that could move the
// cursor or rewrite what a terminal shows: C0 but newline and tab, DEL,
// and C1, whether in UTF-8 or as a lone byte, and the bidi controls
// (U+202E and its kin) that reorder what follows them. It's for text a VM or a
// build made (its output, a path, a host), and every message value goes
// through OneLine, its one-line form, when the message renders. Other
// bytes, invalid UTF-8 among them, stay as they are.
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
		case unicode.IsControl(r), unicode.Is(unicode.Bidi_Control, r):
		default:
			b.WriteString(s[i : i+n])
		}
		i += n
	}
	return b.String()
}
