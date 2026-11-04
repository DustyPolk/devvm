package ui

import (
	"strings"
	"unicode/utf8"

	"github.com/w3cj/devvm/internal/msg"
)

// The SGR codes devvm colors with: only the terminal's 16 base colors, which
// its theme maps to colors that read well on its background, and bold, dim
// and underline. No bright colors (Solarized maps them to greys, and bright
// black is Solarized Dark's background), no backgrounds, no 256 or truecolor.
const (
	sgrBold  = "1"
	sgrDim   = "2"
	sgrError = "1;31" // Die's devvm: label, bold red
	sgrWarn  = "1;33" // Warn's devvm: label, bold yellow
	sgrLink  = "4;34" // doc links, underlined blue
	sgrDone  = "32"   // the done mark, green
)

// A style colors what devvm prints on one stream: labels, doc links and
// log text, never a message's own text. With names, it shows messages as
// their names (no_vm dir=/x) in place of their text, without doc links. The
// zero style is plain text.
type style struct{ on, names bool }

// newStyle is the style of a stream read with getenv: colored as colorOn
// says, in names mode with DEVVM_MESSAGES=names. It's the only place devvm
// reads names mode.
func newStyle(getenv func(string) string, terminal bool) style {
	return style{on: colorOn(getenv, terminal), names: namesMode(getenv)}
}

func namesMode(getenv func(string) string) bool { return getenv("DEVVM_MESSAGES") == "names" }

// segments is m's text in segments and its doc link, as Segments has them,
// or its names as one segment and no link in names mode.
func (s style) segments(m msg.M) ([]msg.Segment, string) {
	if s.names {
		return []msg.Segment{{Text: m.Names()}}, ""
	}
	return m.Segments()
}

// line is m on one line without its doc link: its text, or its names in
// names mode. The zero message is "".
func (s style) line(m msg.M) string {
	switch {
	case m.ID() == "":
		return ""
	case s.names:
		return m.Names()
	}
	return m.Text()
}

// colorOn reports whether a stream gets color: never with NO_COLOR or
// DEVVM_MESSAGES=names, always with FORCE_COLOR or CLICOLOR_FORCE (but 0),
// never with TERM=dumb, else only when the stream is a terminal.
func colorOn(getenv func(string) string, terminal bool) bool {
	switch {
	case getenv("NO_COLOR") != "", namesMode(getenv):
		return false
	case forced(getenv("FORCE_COLOR")), forced(getenv("CLICOLOR_FORCE")):
		return true
	case getenv("TERM") == "dumb":
		return false
	}
	return terminal
}

func forced(v string) bool { return v != "" && v != "0" }

// paint wraps text in the SGR code, then a reset, when the style is on.
func (s style) paint(code, text string) string {
	if !s.on || code == "" || text == "" {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

// paintLines paints each line of text apart, so a line shown on its own
// (scrolled, or copied) carries its code and its reset.
func (s style) paintLines(code, text string) string {
	if !s.on {
		return text
	}
	var b strings.Builder
	for line := range strings.SplitAfterSeq(text, "\n") {
		body := strings.TrimSuffix(line, "\n")
		b.WriteString(s.paint(code, body) + line[len(body):])
	}
	return b.String()
}

// message renders m as devvm prints it: its text, its values bold, then
// its doc link, colored, on a line of its own.
func (s style) message(m msg.M) string {
	segs, link := s.segments(m)
	var b strings.Builder
	for _, seg := range segs {
		if seg.Value {
			b.WriteString(s.paintLines(sgrBold, seg.Text))
		} else {
			b.WriteString(seg.Text)
		}
	}
	if link != "" {
		b.WriteString("\n" + s.paint(sgrLink, link))
	}
	return b.String()
}

// valueParts is m on one line, as line has it, in parts: its values bold.
func (s style) valueParts(m msg.M) []part {
	segs, _ := s.segments(m)
	parts := make([]part, 0, len(segs))
	for _, seg := range segs {
		p := part{text: seg.Text}
		if seg.Value {
			p.code = sgrBold
		}
		parts = append(parts, p)
	}
	return parts
}

// plainText is the parts' text, unpainted.
func plainText(parts []part) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.text)
	}
	return b.String()
}

// labelled renders m with a "devvm: " label, painted with code, before
// each line: Warn's and Die's.
func (s style) labelled(code string, m msg.M) string {
	return prefixLines(s.paint(code, "devvm:")+" ", s.message(m))
}

// A part is some of a line to paint with code ("" for none).
type part struct{ text, code string }

// fit renders as much of the parts as fits in n runes, each painted, and
// how many runes it shows: it counts and cuts the plain text only.
func (s style) fit(parts []part, n int) (string, int) {
	var b strings.Builder
	shown := 0
	for _, p := range parts {
		t := truncate(p.text, n-shown)
		shown += utf8.RuneCountInString(t)
		b.WriteString(s.paint(p.code, t))
	}
	return b.String(), shown
}
