package platform

import "context"

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
