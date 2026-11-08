//go:build unix

package platform

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunClipboard(t *testing.T) {
	for _, tt := range []struct {
		name   string
		script string
		want   string
		err    error  // nil: none, unless says is set
		says   string // in the error of a failure that means nothing more
	}{
		{name: "output", script: `printf 'PNG'`, want: "PNG"},
		{name: "at the cap", script: `printf '0123456789'`, want: "0123456789"},
		{name: "past the cap", script: `printf '0123456789A'`, err: ErrClipboardTooLarge},
		{name: "past the cap, never ending", script: `exec yes`, err: ErrClipboardTooLarge},
		{name: "empty", script: `echo 'Nothing is copied' >&2; exit 1`, err: ErrNoClipboardImage},
		{name: "no display", script: `echo "Error: Can't open display: (null)" >&2; exit 1`, err: ErrNoClipboardReader},
		{name: "other failure", script: `echo 'it broke' >&2; exit 3`, says: "it broke"},
		{name: "stderr kept short", script: `printf 'x%.0s' $(seq 2000) >&2; exit 3`, says: strings.Repeat("x", clipboardStderr)},
	} {
		start := time.Now()
		got, err := runClipboard(t.Context(), 10, "sh", "-c", tt.script)
		switch {
		case tt.says != "":
			if err == nil || !strings.Contains(err.Error(), tt.says) {
				t.Errorf("%s: got %v, want an error with %q", tt.name, err, tt.says)
			}
			if err != nil && strings.Contains(err.Error(), strings.Repeat("x", clipboardStderr+1)) {
				t.Errorf("%s: kept more than %d bytes of stderr", tt.name, clipboardStderr)
			}
		case string(got) != tt.want || !errors.Is(err, tt.err) || (tt.err == nil) != (err == nil):
			t.Errorf("%s: got %q, %v; want %q, %v", tt.name, got, err, tt.want, tt.err)
		}
		if d := time.Since(start); d > clipboardTimeout/2 {
			t.Errorf("%s: took %v", tt.name, d)
		}
	}
	if _, err := runClipboard(t.Context(), 10, "devvm-no-such-reader"); !errors.Is(err, ErrNoClipboardReader) {
		t.Errorf("a reader that isn't installed: %v, want ErrNoClipboardReader", err)
	}
}
