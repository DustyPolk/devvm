//go:build linux || darwin

package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A clipboard tool's output is kept to its limit, the tool stopped past
// it; a tool that isn't there is ClipToolMissing; one that fails is
// ClipToolFailed with its stderr, at most clipStderrMax of it; and a
// cancelled run is the context's error.
func TestRunClipTool(t *testing.T) {
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	for _, c := range []struct {
		name   string
		ctx    context.Context //nolint:containedctx // each case's own
		tool   string
		script string
		out    string
		check  func(error) bool
	}{
		{"its output", t.Context(), "sh", "printf hello", "hello", func(err error) bool { return err == nil }},
		{"past the limit", t.Context(), "sh", "while :; do echo flood; done", "", func(err error) bool { return errors.Is(err, ErrClipTooLarge) }},
		{"missing", t.Context(), "devvm-no-such-clipboard-tool", "", "", func(err error) bool {
			var missing *ClipToolMissing
			return errors.As(err, &missing) && missing.Tool == "devvm-no-such-clipboard-tool"
		}},
		{"failing", t.Context(), "sh", "echo nope >&2; exit 3", "", func(err error) bool {
			var failed *ClipToolFailed
			return errors.As(err, &failed) && failed.Tool == "sh" && strings.TrimSpace(failed.Stderr) == "nope"
		}},
		{"failing at length", t.Context(), "sh", "i=0; while [ $i -lt 2000 ]; do printf 'xxxxx' >&2; i=$((i+1)); done; exit 1", "", func(err error) bool {
			var failed *ClipToolFailed
			return errors.As(err, &failed) && len(failed.Stderr) == clipStderrMax
		}},
		{"cancelled", cancelled, "sh", "sleep 5", "", func(err error) bool { return errors.Is(err, context.Canceled) }},
	} {
		out, err := runClipTool(c.ctx, c.tool, []string{"-c", c.script}, 64)
		if string(out) != c.out || !c.check(err) {
			t.Errorf("%s: %q, %v", c.name, out, err)
		}
	}
}
