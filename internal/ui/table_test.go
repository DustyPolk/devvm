package ui

import (
	"fmt"
	"strings"
	"testing"
	"text/tabwriter"
)

// A plain table is what text/tabwriter writes with padding 2, so tables
// devvm printed with it stay byte for byte the same; a colored one is the
// same once its codes are taken out.
func TestTableLikeTabwriter(t *testing.T) {
	t.Parallel()
	rows := [][]string{
		{"NAME", "STATE", "SIZE", "PATH"},
		{"base", "current", "1.0G", "/a"},
		{"docker+foo", "stale", "-", "/über/long path"},
		{"é", "unfinished", "12K", "-"},
	}
	var want strings.Builder
	w := tabwriter.NewWriter(&want, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		_, _ = fmt.Fprintln(w, strings.Join(r, "\t"))
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{false, true} {
		var out strings.Builder
		tab := &Table{w: &out, st: style{on: on}}
		tab.Header(rows[0]...)
		tab.Row(Cell{"base", Plain}, Cell{"current", Good}, Cell{"1.0G", Plain}, Cell{"/a", Secondary})
		tab.Row(Cell{"docker+foo", Plain}, Cell{"stale", Attention}, Cell{"-", Plain}, Cell{"/über/long path", Plain})
		tab.Total(plainCells(rows[3]...)...)
		if err := tab.Flush(); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if on {
			for _, painted := range []string{"\033[1mNAME\033[0m  ", "\033[32mcurrent\033[0m", "\033[33mstale\033[0m", "\033[2m/a\033[0m", "\033[1munfinished\033[0m"} {
				if !strings.Contains(got, painted) {
					t.Errorf("colored: no %q in %q", painted, got)
				}
			}
			got = plain(got)
		}
		if got != want.String() {
			t.Errorf("color %v: got\n%q\nwant\n%q", on, got, want.String())
		}
	}
}

// Break ends a table: the next lines up on its own, after a blank line.
func TestTableBreak(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	tab := &Table{w: &out}
	tab.Header("A", "B")
	tab.Row(plainCells("long name", "x")...)
	if err := tab.Break(); err != nil {
		t.Fatal(err)
	}
	tab.Header("C", "D")
	if err := tab.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "A          B\nlong name  x\n\nC  D\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A cell's text is shown on one line, its control and bidi characters
// dropped: a VM's folder with a newline or an escape never breaks a row
// or moves the cursor.
func TestTableCellsOneLine(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	tab := &Table{w: &out}
	tab.Row(Cell{Text: "devvm-x\033[2J"}, Cell{Text: "/p/a\nb" + string(rune(0x202e))})
	tab.Total(Cell{Text: "\tall"}, Cell{Text: "1\r"})
	if err := tab.Flush(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "devvm-x[2J  /p/ab\nall         1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A UI's table is colored as its stdout is.
func TestUITable(t *testing.T) {
	t.Parallel()
	u, out, _ := testUI(t, "")
	u.outStyle = style{on: true}
	tab := u.Table()
	tab.Header("A")
	if err := tab.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "\033[1mA\033[0m\n" {
		t.Errorf("got %q", got)
	}
}
