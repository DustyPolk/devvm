package clipboard

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"runtime"
	"testing"
)

// pngOf is a w by h PNG, its first pixel red.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// exifMarker is in the EXIF segment of jpegWithEXIF, as a photo's location
// would be.
const (
	exifMarker  = "GPS 51.5007N 0.1246W"
	exifPayload = "Exif\x00\x00" + exifMarker
)

// jpegWithEXIF is a 4 by 3 JPEG with an EXIF (APP1) segment after its
// start.
func jpegWithEXIF(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 4, 3)), nil); err != nil {
		t.Fatal(err)
	}
	app1 := binary.BigEndian.AppendUint16([]byte{0xff, 0xe1}, uint16(len(exifPayload)+2))
	raw := b.Bytes()
	return bytes.Join([][]byte{raw[:2], app1, []byte(exifPayload), raw[2:]}, nil)
}

// bombPNG is a small PNG whose header says it's w by h, its checksum
// right, so only its size gives it away.
func bombPNG(t *testing.T, w, h uint32) []byte {
	t.Helper()
	raw := pngOf(t, 1, 1)
	// The signature (8 bytes), then IHDR: its length, its type, its width
	// and height first in its 13 bytes, then its CRC of type and data.
	binary.BigEndian.PutUint32(raw[16:], w)
	binary.BigEndian.PutUint32(raw[20:], h)
	binary.BigEndian.PutUint32(raw[29:], crc32.ChecksumIEEE(raw[12:29]))
	return raw
}

func TestSanitize(t *testing.T) {
	good := pngOf(t, 5, 7)
	for _, tt := range []struct {
		name string
		raw  []byte
		err  error // nil: a PNG of bounds
		w, h int
	}{
		{name: "png", raw: good, w: 5, h: 7},
		{name: "png with something after its end", raw: append(bytes.Clone(good), "#!/bin/sh\nrm -rf ~\n"...), w: 5, h: 7},
		{name: "jpeg with exif", raw: jpegWithEXIF(t), w: 4, h: 3},
		{name: "text", raw: []byte("hello, world\n"), err: ErrNotImage},
		{name: "empty", raw: nil, err: ErrNotImage},
		{name: "gif", raw: []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), err: ErrNotImage},
		{name: "truncated png", raw: good[:len(good)/2], err: ErrNotImage},
		{name: "png header alone", raw: good[:8], err: ErrNotImage},
		{name: "too wide", raw: bombPNG(t, maxSide+1, 1), err: ErrTooLarge},
		{name: "too many pixels", raw: bombPNG(t, maxSide, maxSide), err: ErrTooLarge},
		{name: "over MaxBytes", raw: append(bytes.Clone(good), make([]byte, MaxBytes)...), err: ErrTooLarge},
	} {
		out, err := Sanitize(tt.raw)
		if tt.err != nil {
			if !errors.Is(err, tt.err) {
				t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		img, err := png.Decode(bytes.NewReader(out))
		if err != nil {
			t.Errorf("%s: the output isn't a PNG: %v", tt.name, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != tt.w || b.Dy() != tt.h {
			t.Errorf("%s: bounds %v, want %dx%d", tt.name, b, tt.w, tt.h)
		}
		if bytes.Contains(out, []byte(exifMarker)) || bytes.Contains(out, []byte("Exif")) || bytes.Contains(out, []byte("rm -rf")) {
			t.Errorf("%s: the output kept what wasn't pixels", tt.name)
		}
	}
}

// A header's size is refused before anything is decoded: a 100000 by
// 100000 image would be 40 GB.
func TestSanitizeBombAllocatesNothing(t *testing.T) {
	raw := bombPNG(t, 100000, 100000)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := Sanitize(raw)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("%v, want ErrTooLarge", err)
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
		t.Errorf("allocated %d bytes refusing it", n)
	}
}
