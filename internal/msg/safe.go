package msg

import "unicode"

// Safe reports whether a terminal shows r as it is, so that text from a VM
// (or any other untrusted text) can't move the cursor, reorder, fake or
// hide what the user reads: r is graphic (a letter, mark, number,
// punctuation, symbol or space, in any script, emoji too), and not a
// format character (Cf: zero-width spaces and joiners, the word joiner,
// the soft hyphen, the tag characters), a bidi control, a variation
// selector or one of the fillers that show as nothing (Hangul's, the
// braille blank, the combining grapheme joiner). Controls, tab and
// newline among them, aren't safe: a caller that keeps lines keeps
// those itself. It's the one rule for which characters reach the
// terminal: Printable and OneLine drop the rest, a diff escapes them.
func Safe(r rune) bool {
	return unicode.IsGraphic(r) &&
		!unicode.Is(unicode.Cf, r) &&
		!unicode.Is(unicode.Bidi_Control, r) &&
		!unicode.Is(unicode.Variation_Selector, r) &&
		!invisible(r)
}

// invisible is a graphic character most terminals show as blank space or
// nothing at all, though Unicode doesn't call it a space.
func invisible(r rune) bool {
	switch r {
	case 0x034f, // combining grapheme joiner
		0x115f, 0x1160, // Hangul choseong and jungseong fillers
		0x17b4, 0x17b5, // Khmer's inherent vowels
		0x2800, // braille pattern blank
		0x3164, // Hangul filler
		0xffa0: // halfwidth Hangul filler
		return true
	}
	return false
}
