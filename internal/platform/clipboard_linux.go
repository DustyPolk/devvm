package platform

import (
	"context"
	"strings"
)

// clipboardTypesLimit is the most of a reader's list of types devvm reads:
// a list is a few lines.
const clipboardTypesLimit = 64 << 10

// ClipboardImage returns the image on this machine's clipboard, at most
// limit bytes, as its reader gives it: PNG when the clipboard offers it,
// else JPEG. It reads through wl-paste under Wayland (WAYLAND_DISPLAY),
// else xclip under X11 (DISPLAY), getenv reading those.
func ClipboardImage(ctx context.Context, getenv func(string) string, limit int64) ([]byte, error) {
	return clipboardImage(ctx, getenv, limit, runClipboard)
}

func clipboardImage(ctx context.Context, getenv func(string) string, limit int64, run clipboardRun) ([]byte, error) {
	var name string
	var list []string
	var read func(typ string) []string
	switch {
	case getenv("WAYLAND_DISPLAY") != "":
		name, list = "wl-paste", []string{"--list-types"}
		read = func(typ string) []string { return []string{"--no-newline", "--type", typ} }
	case getenv("DISPLAY") != "":
		name, list = "xclip", []string{"-selection", "clipboard", "-t", "TARGETS", "-o"}
		read = func(typ string) []string { return []string{"-selection", "clipboard", "-t", typ, "-o"} }
	default:
		return nil, ErrNoClipboardReader
	}
	types, err := run(ctx, clipboardTypesLimit, name, list...)
	if err != nil {
		return nil, err
	}
	typ := pickClipboardType(string(types))
	if typ == "" {
		return nil, ErrNoClipboardImage
	}
	return run(ctx, limit, name, read(typ)...)
}

// pickClipboardType picks the type to read from a reader's list of the
// clipboard's types, one a line: image/png, else image/jpeg, as listed,
// parameters and all, since the reader matches the type whole; "" for
// neither.
func pickClipboardType(list string) string {
	lines := strings.Split(list, "\n")
	for _, want := range []string{"image/png", "image/jpeg"} {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			base, _, _ := strings.Cut(line, ";")
			if strings.EqualFold(strings.TrimSpace(base), want) {
				return line
			}
		}
	}
	return ""
}
