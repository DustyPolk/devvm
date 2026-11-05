package vm

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// diffContext is how many unchanged lines a diff shows around each change.
const diffContext = 3

// diffCells is the most lines of one file times the other's that a diff
// compares line by line (after their common start and end); larger
// changes show every line of one as taken out and the other's as added.
const diffCells = 1 << 20

// diffMaxBytes and diffMaxLines are the most of a file, before and
// after together, that lineDiff shows; a larger one is a line saying so.
const (
	diffMaxBytes = 1 << 20
	diffMaxLines = 10000
)

// lineDiff returns a unified diff of a file's text before and after, its
// lines without their line ends: hunks of the changed lines, each with
// diffContext lines around it and its @@ header. Every line is
// shownLine's, as a VM's text shown as data is. None when they're the
// same; one line saying so when they're too long to show.
func lineDiff(before, after string) []string {
	if before == after {
		return nil
	}
	if n := strings.Count(before, "\n") + strings.Count(after, "\n"); len(before)+len(after) > diffMaxBytes || n > diffMaxLines {
		return []string{"  (" + strconv.Itoa(n) + " lines before and after, too long to show)"}
	}
	a, b := splitLines(before), splitLines(after)
	ops := diffOps(a, b)
	var out []string
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		start := max(0, i-diffContext)
		end := i
		for end < len(ops) {
			if ops[end].kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*diffContext {
				end = min(end+diffContext, len(ops))
				break
			}
			end = run
		}
		out = append(out, hunk(ops[start:end])...)
		i = end
	}
	return out
}

// diffOp is one line of a diff: kept (' '), taken out ('-') or added
// ('+'), with its line numbers in old and new (1-based, of the line it's
// at or after).
type diffOp struct {
	kind   byte
	line   string
	at, to int
}

// hunk is ops' lines with their @@ header.
func hunk(ops []diffOp) []string {
	var olds, news int
	for _, o := range ops {
		if o.kind != '+' {
			olds++
		}
		if o.kind != '-' {
			news++
		}
	}
	at, to := ops[0].at, ops[0].to
	if olds == 0 {
		at--
	}
	if news == 0 {
		to--
	}
	out := []string{"@@ -" + strconv.Itoa(at) + "," + strconv.Itoa(olds) + " +" + strconv.Itoa(to) + "," + strconv.Itoa(news) + " @@"}
	for _, o := range ops {
		out = append(out, string(o.kind)+shownLine(o.line))
	}
	return out
}

// diffOps is a and b's lines as kept, taken out and added, in order: their
// common start and end kept, and between them the longest common
// subsequence's lines kept, when it's small enough to find (diffCells).
func diffOps(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	ma, mb := a[pre:len(a)-suf], b[pre:len(b)-suf]
	var ops []diffOp
	i, j := 0, 0
	add := func(kind byte, line string) {
		ops = append(ops, diffOp{kind: kind, line: line, at: i + 1, to: j + 1})
		if kind != '+' {
			i++
		}
		if kind != '-' {
			j++
		}
	}
	for _, l := range a[:pre] {
		add(' ', l)
	}
	for _, o := range middleOps(ma, mb) {
		add(o.kind, o.line)
	}
	for _, l := range a[len(a)-suf:] {
		add(' ', l)
	}
	return ops
}

// middleOps is diffOps' middle, the lines between a and b's common start
// and end, by their longest common subsequence.
func middleOps(a, b []string) []diffOp {
	var ops []diffOp
	if len(a)*len(b) > diffCells {
		for _, l := range a {
			ops = append(ops, diffOp{kind: '-', line: l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{kind: '+', line: l})
		}
		return ops
	}
	// lcs[i][j] is the longest common subsequence of a[i:] and b[j:].
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			ops = append(ops, diffOp{kind: ' ', line: a[i]})
			i++
			j++
		case i < len(a) && (j == len(b) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, diffOp{kind: '-', line: a[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: '+', line: b[j]})
			j++
		}
	}
	return ops
}

// shownLine is a VM's text as a diff shows it, on one line: a tab and
// every character a terminal shows as it is, a backslash doubled, and
// every other character, such as a control, format, bidi or zero-width
// character, a soft hyphen or Unicode's tag characters, written out as
// \u{HEX} (a byte that isn't UTF-8 as \x{HEX}), so nothing in it hides
// or changes what the diff shows.
func shownLine(s string) string {
	const softHyphen = 0xad // graphic to Unicode, but most terminals show nothing
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			b.WriteString(`\x{` + strconv.FormatUint(uint64(s[i]), 16) + "}")
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\t' || unicode.IsGraphic(r) && r != softHyphen:
			b.WriteRune(r)
		default:
			b.WriteString(`\u{` + strconv.FormatInt(int64(r), 16) + "}")
		}
		i += n
	}
	return b.String()
}

// splitLines is s's lines without their line ends; none for "".
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
