package main

import (
	"encoding/hex"
	"slices"
	"strings"
)

// The clipboard's stubs stand in for the commands devvm reads this
// machine's clipboard image with, never touching the real clipboard: the
// switch clipboard-png is the image on it (none without it), and
// clipboard-fails makes each fail as it does with no display. Each logs
// every call to $STUB_LOG, and fails as the real one does where the image
// isn't there.

// wlPaste stands in for wl-clipboard's wl-paste: --list-types lists
// image/png, and --type image/png prints the image.
func wlPaste(c *call) int {
	c.log()
	img, ok := c.clipboard()
	switch {
	case c.on("clipboard-fails"):
		c.say("Failed to connect to a Wayland server: No such file or directory")
	case slices.Contains(c.args, "--list-types") && ok:
		c.print("image/png\n")
		return 0
	case slices.Contains(c.args, "--list-types"):
		c.say("Nothing is copied")
	case c.typeArg("--type") == "image/png" && ok:
		c.print(string(img))
		return 0
	default:
		c.say(`Clipboard content is not available as requested type "` + c.typeArg("--type") + `"`)
	}
	return 1
}

// xclip stands in for xclip -selection clipboard -o: -t TARGETS lists
// image/png among its targets, and -t image/png prints the image.
func xclip(c *call) int {
	c.log()
	img, ok := c.clipboard()
	switch t := c.typeArg("-t"); {
	case c.on("clipboard-fails"):
		c.say("Error: Can't open display: (null)")
	case t == "TARGETS" && ok:
		c.print("TARGETS\nimage/png\n")
		return 0
	case t == "TARGETS":
		c.print("TARGETS\nUTF8_STRING\n")
		return 0
	case t == "image/png" && ok:
		c.print(string(img))
		return 0
	default:
		c.say("Error: target " + t + " not available")
	}
	return 1
}

// osascript stands in for macOS's osascript asking for the clipboard as
// PNG data: it prints the image as AppleScript's «data PNGf…», in hex.
func osascript(c *call) int {
	c.log()
	img, ok := c.clipboard()
	if !ok || c.on("clipboard-fails") || !strings.Contains(c.joined(), "«class PNGf»") {
		c.say("execution error: Can’t make some data into the expected type. (-1700)")
		return 1
	}
	c.print("«data PNGf" + strings.ToUpper(hex.EncodeToString(img)) + "»\n")
	return 0
}

// clipboard is the image on the stubs' clipboard, and whether there's one.
func (c *call) clipboard() ([]byte, bool) {
	if !c.on("clipboard-png") {
		return nil, false
	}
	return c.raw("clipboard-png", ""), true
}

// typeArg is the value after flag, or its --flag=VALUE form's.
func (c *call) typeArg(flag string) string {
	for i, a := range c.args {
		if a == flag && i+1 < len(c.args) {
			return c.args[i+1]
		}
		if v, ok := strings.CutPrefix(a, flag+"="); ok {
			return v
		}
	}
	return ""
}
