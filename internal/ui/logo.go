package ui

import "strings"

// logoText is devvm's logo, its three lines; logoMark is the prompt's
// mark in it, which the logo's color paints.
const (
	logoText = "╭──────────╮\n" +
		"│ ❯ devvm ╭┤\n" +
		"╰─────────██\n"
	logoMark = "❯"
	sgrBrand = "32" // the logo's mark: green, as the done mark
)

// logo is devvm's logo as the style prints it: the mark green when
// colored, the frame in the terminal's own color.
func (s style) logo() string {
	return strings.Replace(logoText, logoMark, s.paint(sgrBrand, logoMark), 1)
}

// Logo prints devvm's logo to stdout, once a run, only for someone
// watching it (logoShows).
func (u *UI) Logo() {
	if !logoShows(u) || u.logoShown.Swap(true) {
		return
	}
	u.write(u.stdout, u.outStyle.logo())
}

// logoShows reports whether u shows the logo: with stdout a terminal (or
// a test build's PickOnPipe), but not with --no-input, after DataOnly
// (--json) or in names mode.
func logoShows(u *UI) bool {
	return (u.stdoutTTY || u.pickOnPipe.Load()) && !u.noInput.Load() && !u.dataOnly.Load() && !u.outStyle.names
}
