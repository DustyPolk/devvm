package platform

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pngScript is the AppleScript that writes the clipboard's image as a PNG
// to the file its argument names (on run argv), which macOS converts from
// whatever image type is there. It fails when no image is on the
// clipboard, so that failure is the check for one.
const pngScript = `on run argv
	set f to POSIX file (item 1 of argv)
	set d to (the clipboard as «class PNGf»)
	set h to open for access f with write permission
	set eof h to 0
	write d to h
	close access h
end run`

// ReadClip reads the clipboard through osascript: one copied file's path
// (Finder's «class furl»), else the image as a PNG, written to a file in
// a private folder of devvm's, read, and deleted, at most limit bytes
// (ErrClipTooLarge past it). A copied file wins over an image, because
// Finder puts the file's icon on the clipboard as an image beside it.
// Text is never asked for.
func ReadClip(ctx context.Context, limit int64) (Clip, error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", "POSIX path of (the clipboard as «class furl»)").Output()
	switch {
	case err == nil:
		return Clip{File: strings.TrimRight(string(out), "\n")}, nil
	case errors.Is(err, exec.ErrNotFound):
		return Clip{}, &ClipToolMissing{Tool: "osascript"}
	case ctx.Err() != nil:
		return Clip{}, ctx.Err()
	}
	dir, err := os.MkdirTemp("", "devvm-paste-")
	if err != nil {
		return Clip{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	file := filepath.Join(dir, "clip.png")
	if err := exec.CommandContext(ctx, "osascript", "-e", pngScript, file).Run(); err != nil { //nolint:gosec // devvm's own script and its own temporary file
		if ctx.Err() != nil {
			return Clip{}, ctx.Err()
		}
		return Clip{}, ErrClipEmpty
	}
	f, err := os.Open(file) //nolint:gosec,forbidigo // devvm's own file, in a folder of mode 700 it just made
	if err != nil {
		return Clip{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	switch {
	case err != nil:
		return Clip{}, err
	case int64(len(data)) > limit:
		return Clip{}, ErrClipTooLarge
	}
	if c, ok := imageOf(data, "image/png"); ok {
		return c, nil
	}
	return Clip{}, ErrClipEmpty
}

// CopyText puts text on the clipboard with pbcopy.
func CopyText(ctx context.Context, text string) error {
	cmd := exec.CommandContext(ctx, "pbcopy")
	cmd.Stdin = strings.NewReader(text)
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return &ClipToolMissing{Tool: "pbcopy"}
	}
	return err
}
