package platform

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestParseOSAScriptPNG(t *testing.T) {
	for _, tt := range []struct {
		name  string
		out   string
		limit int64
		want  []byte
		err   error // nil: any error when want is nil
	}{
		{"good", "«data PNGf89504E47»\n", 4, []byte{0x89, 'P', 'N', 'G'}, nil},
		{"lowercase hex, no newline", "«data PNGf89504e47»", 4, []byte{0x89, 'P', 'N', 'G'}, nil},
		{"odd hex", "«data PNGf89504E4»\n", 4, nil, nil},
		{"not hex", "«data PNGf89ZZ»\n", 4, nil, nil},
		{"another class", "«data TIFF4D4D»\n", 4, nil, nil},
		{"no suffix", "«data PNGf89504E47\n", 4, nil, nil},
		{"text", "hello\n", 4, nil, nil},
		{"two newlines", "«data PNGf89504E47»\n\n", 4, nil, nil},
		{"over the cap", "«data PNGf89504E4700»\n", 4, nil, ErrClipboardTooLarge},
	} {
		got, err := parseOSAScriptPNG([]byte(tt.out), tt.limit)
		switch {
		case tt.want != nil && (err != nil || !bytes.Equal(got, tt.want)):
			t.Errorf("%s: got %x, %v; want %x", tt.name, got, err, tt.want)
		case tt.want == nil && err == nil:
			t.Errorf("%s: got %x, want an error", tt.name, got)
		case tt.err != nil && !errors.Is(err, tt.err):
			t.Errorf("%s: got %v, want %v", tt.name, err, tt.err)
		}
	}
}

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
