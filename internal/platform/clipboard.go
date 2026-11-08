package platform

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
)

// ClipboardImage's errors: the clipboard holds no image; nothing here can
// read it (no reader tool, or no display to read it from); its image is
// over the size asked for.
var (
	ErrNoClipboardImage  = errors.New("no image on the clipboard")
	ErrNoClipboardReader = errors.New("no clipboard reader")
	ErrClipboardTooLarge = errors.New("the clipboard's image is too large")
)

// clipboardFailure is what a reader's failure means, from what it printed
// on stderr: an empty clipboard, or one without the type asked for, has no
// image; a display it can't reach leaves nothing to read from; nil for
// anything else.
func clipboardFailure(stderr string) error {
	for _, f := range []struct {
		says string
		err  error
	}{
		{"Nothing is copied", ErrNoClipboardImage},                      // wl-paste, an empty clipboard
		{"not available", ErrNoClipboardImage},                          // wl-paste and xclip, a type not offered
		{"(-1700)", ErrNoClipboardImage},                                // osascript, no PNG data to coerce
		{"Failed to connect to a Wayland server", ErrNoClipboardReader}, // wl-paste
		{"Can't open display", ErrNoClipboardReader},                    // xclip
	} {
		if strings.Contains(stderr, f.says) {
			return f.err
		}
	}
	return nil
}

// osascript prints a clipboard's PNG data as an AppleScript data literal,
// its bytes in hex.
const (
	osascriptPNGPrefix = "«data PNGf"
	osascriptPNGSuffix = "»"
)

// parseOSAScriptPNG returns the bytes of osascript's «data PNGf<hex>»,
// refusing anything else, and more than limit bytes before decoding them.
func parseOSAScriptPNG(out []byte, limit int64) ([]byte, error) {
	out = bytes.TrimSuffix(out, []byte("\n"))
	h, ok := bytes.CutPrefix(out, []byte(osascriptPNGPrefix))
	if ok {
		h, ok = bytes.CutSuffix(h, []byte(osascriptPNGSuffix))
	}
	if !ok {
		return nil, errors.New("osascript's answer isn't PNG data")
	}
	if int64(len(h)) > 2*limit {
		return nil, ErrClipboardTooLarge
	}
	data := make([]byte, hex.DecodedLen(len(h)))
	if _, err := hex.Decode(data, h); err != nil {
		return nil, errors.New("osascript's PNG data isn't hex")
	}
	return data, nil
}
