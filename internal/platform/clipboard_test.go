package platform

import (
	"errors"
	"strings"
	"testing"
)

// What the readers print when they fail, as this machine's printed it.
func TestClipboardFailure(t *testing.T) {
	for stderr, want := range map[string]error{
		"Nothing is copied\n": ErrNoClipboardImage,
		"Clipboard content is not available as requested type \"image/png\"\n":          ErrNoClipboardImage,
		"Error: target image/png not available\n":                                       ErrNoClipboardImage,
		"Error: target TARGETS not available\n":                                         ErrNoClipboardImage,
		"0:43: execution error: Can’t make some data into the expected type. (-1700)\n": ErrNoClipboardImage,
		"Failed to connect to a Wayland server: No such file or directory\n":            ErrNoClipboardReader,
		"Error: Can't open display: (null)\n":                                           ErrNoClipboardReader,
		"segmentation fault\n":                                                          nil,
		"":                                                                              nil,
	} {
		if got := clipboardFailure(stderr); !errors.Is(got, want) || (want == nil) != (got == nil) {
			t.Errorf("clipboardFailure(%q) = %v, want %v", strings.TrimSpace(stderr), got, want)
		}
	}
}
