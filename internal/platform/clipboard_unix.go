//go:build unix

package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// clipboardTimeout bounds each reader command, so one stuck on a display
// that doesn't answer holds up a paste no longer than this.
const clipboardTimeout = 5 * time.Second

// clipboardStderr is how much of a reader's stderr is kept, for its
// failure's error.
const clipboardStderr = 512

// clipboardRun runs a reader command, name with args, and returns at most
// limit bytes of its output: runClipboard, or a test's stand-in.
type clipboardRun func(ctx context.Context, limit int64, name string, args ...string) ([]byte, error)

// runClipboard runs a reader and returns its output: ErrClipboardTooLarge
// past limit bytes, ErrNoClipboardReader when it isn't installed, and when
// it fails, what its stderr says that means (clipboardFailure). It runs in
// devvm's background processes, with no UI to log to.
func runClipboard(ctx context.Context, limit int64, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, clipboardTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // the readers' own names and arguments
	stderr := &head{max: clipboardStderr}
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNoClipboardReader
		}
		return nil, err
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, limit+1))
	over := int64(len(out)) > limit
	if over {
		cancel() // what's left is never read, so the reader would wait on it
	}
	err = cmd.Wait()
	switch {
	case over:
		return nil, ErrClipboardTooLarge
	case err != nil:
		if failure := clipboardFailure(string(stderr.buf)); failure != nil {
			return nil, failure
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(stderr.buf)))
	case readErr != nil:
		return nil, readErr
	}
	return out, nil
}

// head keeps the first max bytes written to it and takes the rest without
// keeping it, so a command's output can't grow devvm's memory.
type head struct {
	buf []byte
	max int
}

func (h *head) Write(p []byte) (int, error) {
	if room := h.max - len(h.buf); room > 0 {
		h.buf = append(h.buf, p[:min(len(p), room)]...)
	}
	return len(p), nil
}
