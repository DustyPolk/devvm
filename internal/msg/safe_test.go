package msg

import "testing"

// Each class of character, by code point, so this file holds none of the
// invisible ones.
func TestSafe(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		what string
		r    rune
		safe bool
	}{
		{"letter", 'a', true},
		{"space", ' ', true},
		{"accented", 'é', true},
		{"CJK", '漢', true},
		{"full-width letter", 0xff21, true},
		{"emoji", 0x1f600, true},
		{"combining accent", 0x0301, true},
		{"ideographic space", 0x3000, true},
		{"newline", '\n', false},
		{"tab", '\t', false},
		{"escape", 0x1b, false},
		{"DEL", 0x7f, false},
		{"C1 CSI", 0x9b, false},
		{"RLO", 0x202e, false},
		{"LRI", 0x2066, false},
		{"RLM", 0x200f, false},
		{"ALM", 0x061c, false},
		{"zero-width space", 0x200b, false},
		{"zero-width non-joiner", 0x200c, false},
		{"zero-width joiner", 0x200d, false},
		{"word joiner", 0x2060, false},
		{"invisible times", 0x2062, false},
		{"BOM", 0xfeff, false},
		{"soft hyphen", 0x00ad, false},
		{"tag A", 0xe0041, false},
		{"cancel tag", 0xe007f, false},
		{"variation selector 16", 0xfe0f, false},
		{"variation selector 17", 0xe0100, false},
		{"Mongolian vowel separator", 0x180e, false},
		{"combining grapheme joiner", 0x034f, false},
		{"Hangul filler", 0x3164, false},
		{"halfwidth Hangul filler", 0xffa0, false},
		{"Hangul choseong filler", 0x115f, false},
		{"Khmer inherent vowel", 0x17b4, false},
		{"braille blank", 0x2800, false},
		{"line separator", 0x2028, false},
		{"private use", 0xe000, false},
		{"unassigned", 0x0378, false},
	} {
		if got := Safe(tc.r); got != tc.safe {
			t.Errorf("Safe(%s, U+%04X) = %v, want %v", tc.what, tc.r, got, tc.safe)
		}
	}
}

// OneLine drops what Safe refuses, and keeps the rest.
func TestOneLineDropsInvisible(t *testing.T) {
	t.Parallel()
	in := "a" + string(rune(0x200b)) + "b" + string(rune(0xe0041)) + "c" + string(rune(0x3164)) +
		"d" + string(rune(0x2800)) + "é漢" + string(rune(0x1f600))
	want := "abcdé漢" + string(rune(0x1f600))
	if got := OneLine(in); got != want {
		t.Errorf("OneLine(%q) = %q, want %q", in, got, want)
	}
}
