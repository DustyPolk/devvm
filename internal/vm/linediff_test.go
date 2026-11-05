package vm

import (
	"slices"
	"strings"
	"testing"
)

// lineDiff is a unified diff: hunks with three lines around each change,
// changes close together in one hunk, a new file and an emptied one, and
// a VM's control characters never reaching the terminal.
func TestLineDiff(t *testing.T) {
	lines := func(n int, change map[int]string) string {
		var b strings.Builder
		for i := 1; i <= n; i++ {
			if s, ok := change[i]; ok {
				b.WriteString(s + "\n")
				continue
			}
			b.WriteString("l" + string(rune('a'+i%26)) + "\n")
		}
		return b.String()
	}
	for _, tc := range []struct {
		name          string
		before, after string
		want          []string
	}{
		{"same", "a\nb\n", "a\nb\n", nil},
		{"new", "", "a\nb\n", []string{"@@ -0,0 +1,2 @@", "+a", "+b"}},
		{"emptied", "a\n", "", []string{"@@ -1,1 +0,0 @@", "-a"}},
		{"middle", "a\nb\nc\n", "a\nx\nc\n", []string{"@@ -1,3 +1,3 @@", " a", "-b", "+x", " c"}},
		{"added", "a\nc\n", "a\nb\nc\n", []string{"@@ -1,2 +1,3 @@", " a", "+b", " c"}},
		{
			"apart", lines(20, nil), lines(20, map[int]string{2: "X", 18: "Y"}),
			[]string{
				"@@ -1,5 +1,5 @@", " lb", "-lc", "+X", " ld", " le", " lf",
				"@@ -15,6 +15,6 @@", " lp", " lq", " lr", "-ls", "+Y", " lt", " lu",
			},
		},
		{
			"close", lines(12, nil), lines(12, map[int]string{2: "X", 8: "Y"}),
			[]string{"@@ -1,11 +1,11 @@", " lb", "-lc", "+X", " ld", " le", " lf", " lg", " lh", "-li", "+Y", " lj", " lk", " ll"},
		},
		{"control", "a\n", "a\x1b[31mred\x07\n", []string{"@@ -1,1 +1,1 @@", "-a", `+a\u{1b}[31mred\u{7}`}},
		{
			"hidden", "a\n", string([]rune{0xe0041, 0x200b, 'b', 0x202e, 'c', 0xad}) + "\\u{1}\xff\td\n",
			[]string{"@@ -1,1 +1,1 @@", "-a", `+\u{e0041}\u{200b}b\u{202e}c\u{ad}\\u{1}\x{ff}` + "\td"},
		},
		{"too long", strings.Repeat("a\n", diffMaxLines), "b\n", []string{"  (10001 lines before and after, too long to show)"}},
		{"too big", strings.Repeat("a", diffMaxBytes), "b", []string{"  (0 lines before and after, too long to show)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := lineDiff(tc.before, tc.after); !slices.Equal(got, tc.want) {
				t.Errorf("lineDiff =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tc.want, "\n"))
			}
		})
	}
}

// Files too big to compare line by line show one whole as taken out and
// the other as added.
func TestLineDiffTooBig(t *testing.T) {
	var a, b []string
	for i := range 1100 {
		a = append(a, "a"+string(rune('0'+i%10)))
		b = append(b, "b"+string(rune('0'+i%10)))
	}
	got := lineDiff(strings.Join(a, "\n")+"\n", strings.Join(b, "\n")+"\n")
	if len(got) != 1+2200 || got[0] != "@@ -1,1100 +1,1100 @@" || got[1] != "-a0" || got[1101] != "+b0" {
		t.Errorf("lineDiff = %d lines, %v", len(got), got[:3])
	}
}
