package platform

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestPickClipboardType(t *testing.T) {
	for list, want := range map[string]string{
		"text/plain\nimage/jpeg\nimage/png\n":     "image/png",
		"image/jpeg\ntext/plain\n":                "image/jpeg",
		"TARGETS\nimage/png;charset=binary\n":     "image/png;charset=binary",
		"  IMAGE/PNG  \n":                         "IMAGE/PNG",
		"text/plain\nimage/gif\nimage/webp\n":     "",
		"image/pngx\nimage/png-ish\n":             "",
		"":                                        "",
		"UTF8_STRING\nSTRING\ntext/plain;utf-8\n": "",
	} {
		if got := pickClipboardType(list); got != want {
			t.Errorf("pickClipboardType(%q) = %q, want %q", list, got, want)
		}
	}
}

// fakeReaders stands in for wl-paste and xclip: types is the list of types
// they print, image what they print for a type, fail a call's error.
type fakeReaders struct {
	types string
	image string
	fail  map[string]error // by the call, as calls has it
	calls []string
}

func (f *fakeReaders) run(_ context.Context, _ int64, name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	f.calls = append(f.calls, call)
	if err := f.fail[call]; err != nil {
		return nil, err
	}
	if slices.Contains(args, "TARGETS") || slices.Contains(args, "--list-types") {
		return []byte(f.types), nil
	}
	return []byte(f.image), nil
}

func TestClipboardImage(t *testing.T) {
	wayland := map[string]string{"WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":0"}
	x11 := map[string]string{"DISPLAY": ":0"}
	for _, tt := range []struct {
		name  string
		env   map[string]string
		f     fakeReaders
		want  string
		err   error
		calls []string
	}{
		{
			name: "wayland, png", env: wayland, f: fakeReaders{types: "text/plain\nimage/png\n", image: "PNG"}, want: "PNG",
			calls: []string{"wl-paste --list-types", "wl-paste --no-newline --type image/png"},
		},
		{
			name: "x11, jpeg", env: x11, f: fakeReaders{types: "TARGETS\nimage/jpeg\n", image: "JPEG"}, want: "JPEG",
			calls: []string{"xclip -selection clipboard -t TARGETS -o", "xclip -selection clipboard -t image/jpeg -o"},
		},
		{
			name: "no image type", env: wayland, f: fakeReaders{types: "text/plain\n"}, err: ErrNoClipboardImage,
			calls: []string{"wl-paste --list-types"},
		},
		{
			name: "empty clipboard", env: wayland, f: fakeReaders{fail: map[string]error{"wl-paste --list-types": ErrNoClipboardImage}}, err: ErrNoClipboardImage,
			calls: []string{"wl-paste --list-types"},
		},
		{
			name: "gone before it's read", env: x11, err: ErrNoClipboardImage,
			f:     fakeReaders{types: "image/png\n", fail: map[string]error{"xclip -selection clipboard -t image/png -o": ErrNoClipboardImage}},
			calls: []string{"xclip -selection clipboard -t TARGETS -o", "xclip -selection clipboard -t image/png -o"},
		},
		{name: "no display", env: map[string]string{}, err: ErrNoClipboardReader},
	} {
		got, err := clipboardImage(t.Context(), func(k string) string { return tt.env[k] }, 100, tt.f.run)
		if string(got) != tt.want || !errors.Is(err, tt.err) || (tt.err == nil) != (err == nil) {
			t.Errorf("%s: got %q, %v; want %q, %v", tt.name, got, err, tt.want, tt.err)
		}
		if !slices.Equal(tt.f.calls, tt.calls) {
			t.Errorf("%s: calls %q, want %q", tt.name, tt.f.calls, tt.calls)
		}
	}
}
