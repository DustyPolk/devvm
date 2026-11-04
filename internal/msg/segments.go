package msg

import (
	"os"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// A message's values are marked with these private-use runes while it
// renders for Segments, and the runes are dropped from the values first, so
// a value can't start or end a marking of its own.
const (
	valueStart = ''
	valueEnd   = ''
)

// A Segment is part of a message's text: the template's own words, or a
// value it filled in (a VM's name, a path, a count).
type Segment struct {
	Text  string
	Value bool
}

// Segments returns the message's text in segments, so a terminal can show
// its values apart, and its doc link ("" when it has none). Joined, the
// segments are its Text, but for any marker rune a value held.
func (m M) Segments() (segs []Segment, link string) {
	return m.segmentsIn(loadBundle(), userLanguage(os.Getenv)), m.DocURL()
}

func (m M) segmentsIn(b *i18n.Bundle, lang string) []Segment {
	marked := m
	marked.vals = make([]val, len(m.vals))
	for i, v := range m.vals {
		marked.vals[i] = val{v.key, string(valueStart) + unmarked(v.value) + string(valueEnd)}
	}
	return split(marked.textIn(b, lang))
}

// unmarked is s without the marker runes.
func unmarked(s string) string {
	return strings.Map(func(r rune) rune {
		if r == valueStart || r == valueEnd {
			return -1
		}
		return r
	}, s)
}

// split cuts marked text into segments, the marked parts values; empty
// segments are dropped.
func split(s string) []Segment {
	var segs []Segment
	var b strings.Builder
	value := false
	flush := func() {
		if b.Len() > 0 {
			segs = append(segs, Segment{Text: b.String(), Value: value})
			b.Reset()
		}
	}
	for _, r := range s {
		switch r {
		case valueStart:
			flush()
			value = true
		case valueEnd:
			flush()
			value = false
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return segs
}
