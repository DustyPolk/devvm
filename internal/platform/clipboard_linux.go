package platform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
)

// clipTool is a Linux clipboard tool: wl-paste and wl-copy on Wayland,
// xclip on X11, each a command devvm runs and reads, never logs, so the
// image and the path stay out of the run log.
type clipTool struct {
	paste, copy string
	list        []string                // lists the clipboard's types, one per line
	read        func(t string) []string // reads the clipboard as type t
	write       []string                // copies stdin
	// empty is what the paste tool says on stderr when the clipboard is
	// empty, which is how it reports one: a failed listing saying anything
	// else is the tool failing (ClipToolFailed). nil, where its words
	// aren't known, makes every failed listing an empty clipboard.
	empty []string
}

// linuxClipTool is the clipboard tool for this session: Wayland's when
// WAYLAND_DISPLAY is set, X11's when DISPLAY is, else ErrClipNoSession.
func linuxClipTool() (clipTool, error) {
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		return clipTool{
			paste: "wl-paste", copy: "wl-copy",
			list:  []string{"--list-types"},
			read:  func(t string) []string { return []string{"--no-newline", "--type", t} },
			empty: []string{"Nothing is copied", "No selection"},
		}, nil
	case os.Getenv("DISPLAY") != "":
		return clipTool{
			paste: "xclip", copy: "xclip",
			list:  []string{"-selection", "clipboard", "-t", "TARGETS", "-o"},
			read:  func(t string) []string { return []string{"-selection", "clipboard", "-t", t, "-o"} },
			write: []string{"-selection", "clipboard", "-i"},
		}, nil
	}
	return clipTool{}, ErrClipNoSession
}

// saysEmpty reports whether a failed listing's stderr is the tool's word
// for an empty clipboard.
func (c clipTool) saysEmpty(stderr string) bool {
	if c.empty == nil {
		return true
	}
	for _, s := range c.empty {
		if strings.Contains(stderr, s) {
			return true
		}
	}
	return false
}

// ReadClip reads the clipboard: one copied file's reference, else an
// image, at most limit bytes (ErrClipTooLarge past it), of the types
// devvm sends (chooseTypes), and never text. It lists the types first,
// then reads only the one it chose, so nothing else leaves the clipboard.
// A listing the tool fails with its word for an empty clipboard is one
// (ErrClipEmpty); one it fails otherwise, as wl-paste does when
// WAYLAND_DISPLAY names a session that's gone, is the tool's failure
// (ClipToolFailed), so that copying again isn't the advice.
func ReadClip(ctx context.Context, limit int64) (Clip, error) {
	tool, err := linuxClipTool()
	if err != nil {
		return Clip{}, err
	}
	listed, err := tool.output(ctx, tool.list, 1<<20)
	var failed *ClipToolFailed
	switch {
	case errors.As(err, &failed):
		if tool.saysEmpty(failed.Stderr) {
			return Clip{}, ErrClipEmpty
		}
		return Clip{}, err
	case err != nil && !errors.Is(err, ErrClipTooLarge):
		return Clip{}, err
	}
	for _, t := range chooseTypes(listed) {
		data, err := tool.output(ctx, tool.read(t), limit)
		if err != nil && !errors.As(err, &failed) {
			return Clip{}, err
		}
		if isFileType(t) {
			paths := uriListPaths(data, t == typeGnome)
			switch {
			case len(paths) == 1:
				return Clip{File: paths[0]}, nil
			case len(paths) > 1:
				return Clip{}, &ClipManyFiles{N: len(paths)}
			}
			continue
		}
		if c, ok := imageOf(data, t); ok {
			return c, nil
		}
	}
	return Clip{}, ErrClipEmpty
}

// clipStderrMax is the most of a clipboard tool's stderr devvm keeps.
const clipStderrMax = 4 << 10

// output runs the paste tool with args and returns what it printed, at
// most limit bytes: ErrClipTooLarge past it, with the tool stopped,
// ClipToolMissing when the tool isn't on PATH, and ClipToolFailed, with
// its stderr, when it failed, as wl-paste does on an empty clipboard and
// without a compositor alike.
func (c clipTool) output(ctx context.Context, args []string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.paste, args...) //nolint:gosec // devvm's own arguments: the tool, its switches and a type it listed
	stderr := &capped{max: clipStderrMax}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ClipToolMissing{Tool: c.paste}
		}
		return nil, err
	}
	data, rerr := io.ReadAll(io.LimitReader(out, limit+1))
	if int64(len(data)) > limit {
		cancel()
		_ = cmd.Wait()
		return nil, ErrClipTooLarge
	}
	werr := cmd.Wait()
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case rerr != nil:
		return nil, rerr
	case werr != nil:
		return nil, &ClipToolFailed{Tool: c.paste, Stderr: stderr.String()}
	}
	return data, nil
}

// capped keeps the first max bytes written to it, and takes the rest
// without keeping it. Its buffer is a field, not embedded, so it has no
// ReadFrom, which io.Copy (os/exec's copy of a command's output) would
// read through without end.
type capped struct {
	buf bytes.Buffer
	max int
}

func (w *capped) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// String is what it kept.
func (w *capped) String() string { return w.buf.String() }

// CopyText puts text on the clipboard with wl-copy or xclip. Either
// forks a child that serves the clipboard until something else is copied,
// so the command runs without pipes on its output: Go's Wait would wait
// for that child to close them.
func CopyText(ctx context.Context, text string) error {
	tool, err := linuxClipTool()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, tool.copy, tool.write...) //nolint:gosec // devvm's own arguments
	cmd.Stdin = strings.NewReader(text)
	err = cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return &ClipToolMissing{Tool: tool.copy}
	}
	return err
}
