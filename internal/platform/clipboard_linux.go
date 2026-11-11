package platform

import (
	"context"
	"errors"
	"os"
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

// clipTool is the clipboard tool for this session, found on PATH:
// Wayland's when WAYLAND_DISPLAY is set, X11's when DISPLAY is, else
// ErrClipNoSession.
func (c Clipboard) clipTool() (clipTool, error) {
	switch {
	case os.Getenv("WAYLAND_DISPLAY") != "":
		return clipTool{
			paste: c.tool("wl-paste", ""), copy: c.tool("wl-copy", ""),
			list:  []string{"--list-types"},
			read:  func(t string) []string { return []string{"--no-newline", "--type", t} },
			empty: []string{"Nothing is copied", "No selection"},
		}, nil
	case os.Getenv("DISPLAY") != "":
		return clipTool{
			paste: c.tool("xclip", ""), copy: c.tool("xclip", ""),
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
func (c Clipboard) ReadClip(ctx context.Context, limit int64) (Clip, error) {
	tool, err := c.clipTool()
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
		if clip, ok := imageOf(data, t); ok {
			return clip, nil
		}
	}
	return Clip{}, ErrClipEmpty
}

// isFileType reports whether t is a copied file's reference.
func isFileType(t string) bool { return t == typeURIList || t == typeGnome }

// output runs the paste tool with args (runClipTool), as wl-paste fails on
// an empty clipboard and without a compositor alike.
func (c clipTool) output(ctx context.Context, args []string, limit int64) ([]byte, error) {
	return runClipTool(ctx, c.paste, args, limit)
}

// CopyText puts text on the clipboard with wl-copy or xclip (copyWith),
// which serve it until something else is copied.
func (c Clipboard) CopyText(ctx context.Context, text string) error {
	tool, err := c.clipTool()
	if err != nil {
		return err
	}
	return copyWith(ctx, tool.copy, tool.write, text)
}
