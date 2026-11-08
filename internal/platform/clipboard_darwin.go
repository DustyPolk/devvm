package platform

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os/exec"
)

// ClipboardReader reports whether ClipboardImage has a reader here, without
// reading the clipboard: ErrNoClipboardReader without osascript.
func ClipboardReader(func(string) string) error {
	if _, err := exec.LookPath("osascript"); err != nil {
		return ErrNoClipboardReader
	}
	return nil
}

// ClipboardImage returns the PNG image on this machine's clipboard, at
// most limit bytes, through osascript, which prints it in hex
// (parseOSAScriptPNG). macOS needs no display variable, so getenv goes
// unread.
func ClipboardImage(ctx context.Context, _ func(string) string, limit int64) ([]byte, error) {
	framing := int64(len(osascriptPNGPrefix + osascriptPNGSuffix + "\n"))
	out, err := runClipboard(ctx, 2*limit+framing, "osascript", "-e", "the clipboard as «class PNGf»")
	if err != nil {
		return nil, err
	}
	return parseOSAScriptPNG(out, limit)
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
