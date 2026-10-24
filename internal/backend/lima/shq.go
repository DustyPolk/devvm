package lima

import "strings"

// shq quotes s as one word for a POSIX shell: in single quotes, each single
// quote in s ending the quotes, then a backslash-escaped quote, then opening
// them again. Nothing inside single quotes is special, so any bytes come
// through as they are.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shqJoin quotes each word and joins them with spaces: a command line.
func shqJoin(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = shq(w)
	}
	return strings.Join(q, " ")
}
