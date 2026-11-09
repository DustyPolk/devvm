package main

import (
	"io"
	"os"
	"strings"
)

// The clipboard stubs stand in for the tools devvm paste reads and writes
// this machine's clipboard with: wl-paste and wl-copy (Wayland), xclip
// (X11), osascript and pbcopy (macOS). None touches the real clipboard.
// What's "on the clipboard" is in $STUB_DIR:
//
//	clip-types   the types the clipboard offers, one a line (none: an empty clipboard)
//	clip         the clipboard's contents for any type devvm asks for: an image's bytes, or a
//	             text/uri-list; on macOS, the PNG osascript writes
//	clip-file    on macOS, the copied file's path osascript prints (none: no file copied)
//	clip-floods  a read prints without end, as a tool with a huge image would
//	clip-fails   wl-paste and xclip can't reach the session, as with a WAYLAND_DISPLAY or
//	             DISPLAY from a login that's over, and fail saying so
//	copy-fails   wl-copy, xclip -i and pbcopy fail
//
// What was copied goes to $STUB_DIR/copied, and every call to $STUB_LOG.

// wlPaste stands in for wl-paste: --list-types prints clip-types, and
// fails without it, as wl-paste does on an empty clipboard; --type T
// prints clip when T is listed, else fails.
func wlPaste(c *call) int {
	c.log()
	if c.on("clip-fails") {
		c.say("Failed to connect to a Wayland server")
		return 1
	}
	if c.has("--list-types") {
		return clipTypes(c)
	}
	return clipRead(c, c.after("--type"))
}

// xclip stands in for xclip: -t TARGETS -o prints clip-types, -t T -o
// prints clip when T is listed, and -i copies stdin.
func xclip(c *call) int {
	c.log()
	if c.on("clip-fails") {
		c.say("Error: Can't open display: " + os.Getenv("DISPLAY"))
		return 1
	}
	switch t := c.after("-t"); {
	case c.has("-i"):
		return clipWrite(c)
	case t == "TARGETS":
		return clipTypes(c)
	default:
		return clipRead(c, t)
	}
}

// osascript stands in for macOS's osascript, for devvm paste's two
// scripts alone: one naming «class furl» prints clip-file, and fails
// without it, as AppleScript does when no file is copied; one naming
// «class PNGf» writes clip to the file named by its last argument when
// clip-types lists image/png, and fails otherwise.
func osascript(c *call) int {
	c.log()
	script := c.joined()
	switch {
	case strings.Contains(script, "furl"):
		p, ok := c.text("clip-file")
		if !ok {
			c.say("execution error: Can't make some data into the expected type. (-1700)")
			return 1
		}
		c.print(p + "\n")
		return 0
	case strings.Contains(script, "PNGf"):
		if !clipHas(c, "image/png") {
			c.say("execution error: Can't make some data into the expected type. (-1700)")
			return 1
		}
		return c.status(writeFile(c.last(), c.raw("clip", "")))
	}
	c.say("stub osascript: not one of devvm paste's scripts")
	return 1
}

// clipCopy stands in for wl-copy and pbcopy: it copies stdin to
// $STUB_DIR/copied, and fails with copy-fails.
func clipCopy(c *call) int {
	c.log()
	return clipWrite(c)
}

// clipTypes prints clip-types, failing without it.
func clipTypes(c *call) int {
	t, ok := c.text("clip-types")
	if !ok {
		c.say("Nothing is copied")
		return 1
	}
	c.print(t + "\n")
	return 0
}

// clipHas reports whether clip-types lists t.
func clipHas(c *call, t string) bool {
	types, _ := c.text("clip-types")
	for _, l := range strings.Split(types, "\n") {
		if strings.TrimSpace(l) == t {
			return true
		}
	}
	return false
}

// clipRead prints clip as type t, when t is listed; with clip-floods it
// prints without end.
func clipRead(c *call, t string) int {
	if !clipHas(c, t) {
		c.say("No suitable type of content copied")
		return 1
	}
	if c.on("clip-floods") {
		text := strings.Repeat("\x89PNG\r\n\x1a\nflood", 1<<12)
		for {
			if _, err := io.WriteString(c.stdout, text); err != nil {
				return 1
			}
		}
	}
	c.print(string(c.raw("clip", "")))
	return 0
}

// clipWrite copies stdin to $STUB_DIR/copied, failing with copy-fails.
func clipWrite(c *call) int {
	if c.on("copy-fails") {
		c.drain()
		return 1
	}
	return c.status(c.copyIn("copied"))
}
