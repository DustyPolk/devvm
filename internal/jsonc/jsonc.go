// Package jsonc reads JSON with comments, as devcontainer.json and
// opencode's settings are written: Strip makes it JSON. It is the
// standard library only.
package jsonc

import "strings"

// Strip returns JSON with comments as JSON: its comments, trailing commas
// and byte order mark blanked with spaces, so what's left is JSON at the
// same offsets, newlines kept. Anything inside a string stays as it is.
func Strip(data []byte) []byte {
	out := []byte(string(data))
	if len(out) >= 3 && out[0] == 0xef && out[1] == 0xbb && out[2] == 0xbf {
		out[0], out[1], out[2] = ' ', ' ', ' '
	}
	blank := func(from, to int) {
		for i := from; i < to && i < len(out); i++ {
			if out[i] != '\n' && out[i] != '\r' {
				out[i] = ' '
			}
		}
	}
	// Comments first: a comma before one is still trailing.
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == '"':
			i = stringEnd(out, i)
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '/':
			end := i
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			end := strings.Index(string(out[i+2:]), "*/")
			if end < 0 {
				// Unterminated: the JSON then ends early, and says so.
				blank(i, len(out))
				return out
			}
			blank(i, i+2+end+2)
			i += 2 + end + 1
		}
	}
	for i := 0; i < len(out); i++ {
		switch out[i] {
		case '"':
			i = stringEnd(out, i)
		case ',':
			j := i + 1
			for j < len(out) && strings.IndexByte(" \t\r\n", out[j]) >= 0 {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				out[i] = ' '
			}
		}
	}
	return out
}

// stringEnd returns the index of the quote that ends the string starting
// at start, or the last index when it doesn't end.
func stringEnd(b []byte, start int) int {
	for i := start + 1; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return len(b) - 1
}
