package ui

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/w3cj/devvm/internal/msg"
)

// A Role is what a table cell says, which colors it in a terminal.
type Role int

// The roles a cell can have.
const (
	Plain     Role = iota // names, paths, sizes and counts: the default color
	Good                  // green: current, Running, native
	Attention             // yellow: stale, unused, Stopped, emulated
	Bad                   // red: unreviewed, unenforced
	Secondary             // dim: digests, keys and a feature a backend lacks (none)
)

// sgr is the role's SGR code, "" for none.
func (r Role) sgr() string {
	switch r {
	case Good:
		return "32"
	case Attention:
		return "33"
	case Bad:
		return "31"
	case Secondary:
		return sgrDim
	case Plain:
	}
	return ""
}

// A Cell is a table cell: its text and its role.
type Cell struct {
	Text string
	Role Role
}

// plainCells returns a Plain cell for each text.
func plainCells(texts ...string) []Cell {
	cells := make([]Cell, len(texts))
	for i, t := range texts {
		cells[i] = Cell{Text: t}
	}
	return cells
}

// A Table writes rows of cells in columns two spaces apart, as
// text/tabwriter does with padding 2, every row the same length. In a
// terminal its header and totals are bold and each cell is colored by its
// role; the cells are padded on their plain text first, so the columns
// line up the same with color or without.
type Table struct {
	w    io.Writer
	st   style
	rows []tableRow
}

type tableRow struct {
	cells []Cell
	bold  bool
}

// Table returns a table on stdout, colored when stdout is.
func (u *UI) Table() *Table { return &Table{w: u.Stdout(), st: u.outStyle} }

// Header adds the header row, bold in a terminal.
func (t *Table) Header(names ...string) {
	t.rows = append(t.rows, tableRow{plainCells(names...), true})
}

// Text is m as a cell's text, as the UI's Text has it.
func (t *Table) Text(m msg.M) string { return t.st.line(m) }

// Row adds a row.
func (t *Table) Row(cells ...Cell) { t.rows = append(t.rows, tableRow{cells, false}) }

// Total adds a totals row, bold in a terminal.
func (t *Table) Total(cells ...Cell) { t.rows = append(t.rows, tableRow{cells, true}) }

// Break writes the rows so far, then a blank line, and starts a new
// table, whose columns line up on their own.
func (t *Table) Break() error {
	if err := t.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(t.w, "\n")
	return err
}

// Flush writes the rows so far and starts a new table.
func (t *Table) Flush() error {
	var widths []int
	for _, r := range t.rows {
		for i, c := range r.cells[:max(len(r.cells)-1, 0)] {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
		}
	}
	var b strings.Builder
	for _, r := range t.rows {
		for i, c := range r.cells {
			code := c.Role.sgr()
			if r.bold {
				code = sgrBold
			}
			b.WriteString(t.st.paint(code, c.Text))
			if i < len(r.cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.Text)+2))
			}
		}
		b.WriteString("\n")
	}
	t.rows = nil
	_, err := io.WriteString(t.w, b.String())
	return err
}
