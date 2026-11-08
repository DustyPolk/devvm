package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
)

// MaxBytes is the most devvm reads of the clipboard's image, and the most
// it sends a VM.
const MaxBytes = 32 << 20

// The largest image Sanitize decodes, checked from its header first, so a
// small file can't make it allocate gigabytes (a decode bomb).
const (
	maxSide   = 16384
	maxPixels = 50_000_000
)

// Sanitize's errors: an image over MaxBytes or the largest it decodes; data
// that isn't a PNG or JPEG image.
var (
	ErrTooLarge = errors.New("image too large")
	ErrNotImage = errors.New("not a PNG or JPEG image")
)

// Sanitize decodes a PNG or JPEG image and encodes its pixels again as a
// PNG, which proves it's an image, drops its metadata (a photo's EXIF
// location among it) and anything after its end, and lets no text through.
func Sanitize(raw []byte) ([]byte, error) {
	if len(raw) > MaxBytes {
		return nil, ErrTooLarge
	}
	config, decode := decoders(raw)
	if config == nil {
		return nil, ErrNotImage
	}
	c, err := config(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	if c.Width > maxSide || c.Height > maxSide || c.Width*c.Height > maxPixels {
		return nil, ErrTooLarge
	}
	img, err := decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotImage, err)
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img); err != nil {
		return nil, err
	}
	if out.Len() > MaxBytes {
		return nil, ErrTooLarge
	}
	return out.Bytes(), nil
}

// decoders are the PNG or JPEG decoder for raw, by its first bytes; nil
// for anything else. They're picked here rather than through image's
// registry, which any package in devvm could add a format to.
func decoders(raw []byte) (func(io.Reader) (image.Config, error), func(io.Reader) (image.Image, error)) {
	switch {
	case bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")):
		return png.DecodeConfig, png.Decode
	case bytes.HasPrefix(raw, []byte{0xff, 0xd8}):
		return jpeg.DecodeConfig, jpeg.Decode
	}
	return nil, nil
}
