package main

// shellWords splits a command line into its words as a POSIX shell's
// set -- does, without expanding anything: blanks separate words, single
// quotes keep everything up to the next one, double quotes keep
// everything but a backslash before \, ", $ or `, and a backslash outside
// quotes keeps the next character.
func shellWords(line string) []string {
	var words []string
	var cur []byte
	in := false // a word has begun, maybe empty ('')
	for i := 0; i < len(line); i++ {
		switch ch := line[i]; {
		case ch == ' ' || ch == '\t' || ch == '\n':
			if in {
				words = append(words, string(cur))
				cur, in = nil, false
			}
		case ch == '\'':
			in = true
			for i++; i < len(line) && line[i] != '\''; i++ {
				cur = append(cur, line[i])
			}
		case ch == '"':
			in = true
			for i++; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && isDoubleQuoteEscape(line[i+1]) {
					i++
				}
				cur = append(cur, line[i])
			}
		case ch == '\\' && i+1 < len(line):
			in = true
			i++
			cur = append(cur, line[i])
		default:
			in = true
			cur = append(cur, ch)
		}
	}
	if in {
		words = append(words, string(cur))
	}
	return words
}

// isDoubleQuoteEscape reports whether a backslash before ch in double
// quotes escapes it.
func isDoubleQuoteEscape(ch byte) bool {
	return ch == '\\' || ch == '"' || ch == '$' || ch == '`'
}
