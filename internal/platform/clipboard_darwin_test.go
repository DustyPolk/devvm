package platform

import (
	"bytes"
	"errors"
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
