//go:build linux || darwin

package platform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
)

// clipStderrMax is the most of a clipboard tool's stderr devvm keeps.
const clipStderrMax = 4 << 10

// runClipTool runs the clipboard tool with args and returns what it
// printed, at most limit bytes: ErrClipTooLarge past it, with the tool
// stopped, ClipToolMissing when the tool isn't there, and ClipToolFailed,
// with its stderr, when it failed. Its output is read, never logged, so
// an image and a path stay out of the run log.
func runClipTool(ctx context.Context, tool string, args []string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, args...) //nolint:gosec // devvm's own arguments: the tool, its switches and a type it listed
	stderr := &capped{max: clipStderrMax}
	cmd.Stderr = stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, &ClipToolMissing{Tool: tool}
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
		return nil, &ClipToolFailed{Tool: tool, Stderr: stderr.String()}
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
