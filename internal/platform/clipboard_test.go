package platform

import (
	"slices"
	"testing"
)

// chooseTypes picks a copied file's reference before any image, PNG
// before the other image types, and nothing from text alone.
func TestChooseTypes(t *testing.T) {
	for _, c := range []struct {
		listed string
		want   []string
	}{
		{"text/plain\nSTRING\nUTF8_STRING\n", nil},
		{"", nil},
		{"image/png\ntext/html\n", []string{"image/png"}},
		{"image/webp\nimage/jpeg\nimage/gif\nimage/png\n", []string{"image/png", "image/jpeg", "image/gif", "image/webp"}},
		{"image/png\ntext/uri-list\nx-special/gnome-copied-files\n", []string{"text/uri-list", "x-special/gnome-copied-files", "image/png"}},
		{"TARGETS\r\nimage/bmp\r\nimage/png\r\n", []string{"image/png"}},
		{"image/svg+xml\nimage/bmp\n", nil},
	} {
		if got := chooseTypes([]byte(c.listed)); !slices.Equal(got, c.want) {
			t.Errorf("%q: %q, want %q", c.listed, got, c.want)
		}
	}
}

// uriListPaths reads file: URIs, percent-decoded, past comments and
// blank lines, with or without a localhost, with or without CRLF, and
// GNOME's first line; a web address and another scheme are left out.
func TestURIListPaths(t *testing.T) {
	for _, c := range []struct {
		data  string
		gnome bool
		want  []string
	}{
		{"file:///home/me/shot.png\r\n", false, []string{"/home/me/shot.png"}},
		{"file:///home/me/shot.png\n", false, []string{"/home/me/shot.png"}},
		{"file://localhost/home/me/a%20b.png\n", false, []string{"/home/me/a b.png"}},
		{"# a comment\n\nfile:///a\nfile:///b\n", false, []string{"/a", "/b"}},
		{"https://example.com/x.png\n", false, nil},
		{"file://nas/share/x.png\n", false, nil},
		{"copy\nfile:///home/me/shot.png\n", true, []string{"/home/me/shot.png"}},
		{"cut\nfile:///a\nfile:///b\n", true, []string{"/a", "/b"}},
		{"file:///a\n", true, []string{"/a"}},
		{"copy\n", true, nil},
		{"file:///caf%C3%A9/%E2%9C%93.png\n", false, []string{"/café/✓.png"}},
	} {
		if got := uriListPaths([]byte(c.data), c.gnome); !slices.Equal(got, c.want) {
			t.Errorf("%q gnome=%v: %q, want %q", c.data, c.gnome, got, c.want)
		}
	}
}

// imageOf takes an image whose bytes are the type claimed, and refuses
// bytes of another type or text.
func TestImageOf(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n" + "rest")
	if c, ok := imageOf(png, "image/png"); !ok || c.Type != "image/png" || string(c.Image) != string(png) {
		t.Errorf("a PNG: %v %+v", ok, c)
	}
	if _, ok := imageOf(png, "image/jpeg"); ok {
		t.Error("a PNG as image/jpeg: taken")
	}
	if _, ok := imageOf([]byte("hello"), "image/png"); ok {
		t.Error("text as image/png: taken")
	}
	if _, ok := imageOf([]byte("<svg/>"), "image/svg+xml"); ok {
		t.Error("an SVG: taken")
	}
	if ImageExt("image/jpeg") != "jpg" || ImageExt("image/webp") != "webp" || ImageExt("text/plain") != "" {
		t.Error("ImageExt")
	}
}
