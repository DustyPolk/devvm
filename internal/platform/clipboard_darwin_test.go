package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setScript puts what a test asks for on the pasteboard named by its
// first argument, a test's own, never the clipboard: its second says
// what, the rest are its files. text TEXT; files PATH... (file URLs, one
// item each, with the first's icon as TIFF beside them, as Finder copies);
// ref PATH (a file reference URL, with its icon); image UTI FILE; tiff
// PNGFILE (the PNG made a TIFF); textimage TEXT PNGFILE; clear (nothing);
// release (the pasteboard deleted).
const setScript = `ObjC.import('AppKit');
function run(argv) {
  const pb = $.NSPasteboard.pasteboardWithName(argv[0]);
  if (argv[1] === 'release') { pb.releaseGlobally; return; }
  pb.clearContents;
  const icon = (p) => pb.setDataForType($.NSWorkspace.sharedWorkspace.iconForFile(p).TIFFRepresentation, 'public.tiff');
  switch (argv[1]) {
  case 'text': pb.setStringForType(argv[2], 'public.utf8-plain-text'); break;
  case 'files': {
    const urls = $.NSMutableArray.array;
    for (const p of argv.slice(2)) urls.addObject($.NSURL.fileURLWithPath(p));
    pb.writeObjects(urls);
    icon(argv[2]);
    break;
  }
  case 'ref':
    pb.writeObjects($.NSArray.arrayWithObject($.NSURL.fileURLWithPath(argv[2]).fileReferenceURL));
    icon(argv[2]);
    break;
  case 'image': pb.setDataForType($.NSData.dataWithContentsOfFile(argv[3]), argv[2]); break;
  case 'tiff': pb.setDataForType($.NSBitmapImageRep.imageRepWithData($.NSData.dataWithContentsOfFile(argv[2])).TIFFRepresentation, 'public.tiff'); break;
  case 'textimage':
    pb.setStringForType(argv[2], 'public.utf8-plain-text');
    pb.setDataForType($.NSData.dataWithContentsOfFile(argv[3]), 'public.png');
    break;
  }
}`

// pasteboard is a test's own pasteboard, deleted when the test ends.
type pasteboard struct {
	t    *testing.T
	name string
}

func newPasteboard(t *testing.T) pasteboard {
	t.Helper()
	var b [8]byte
	_, _ = rand.Read(b[:])
	p := pasteboard{t: t, name: "devvm-test-" + hex.EncodeToString(b[:])}
	t.Cleanup(func() { p.set("release") })
	return p
}

// set runs setScript on the pasteboard with args, on the test's context
// without its cancel, as the release runs in its cleanup.
func (p pasteboard) set(args ...string) {
	p.t.Helper()
	argv := append([]string{"-l", "JavaScript", "-e", setScript, p.name}, args...)
	ctx := context.WithoutCancel(p.t.Context())
	if out, err := exec.CommandContext(ctx, "/usr/bin/osascript", argv...).CombinedOutput(); err != nil { //nolint:gosec // G204: the test's own script and arguments
		p.t.Fatalf("setting the pasteboard (%q): %v: %s", args, err, out)
	}
}

// The clipboard's reader on a real pasteboard, a test's own: a copied
// file alone is read as one, Finder's icon beside it ignored, and text is
// never one, whatever path or URL it says; several files are counted; a
// copied file that's gone fails rather than sending its icon; an image is
// read as PNG, a TIFF made one, beside text too; one past the cap is
// refused before it's written.
func TestReadClipPasteboard(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b, gone := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b c.txt"), filepath.Join(dir, "gone.txt")
	for _, f := range []string{a, b, gone} {
		if err := os.WriteFile(f, []byte("text"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	pngFile := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(pngFile, img.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	pb := newPasteboard(t)
	read := func(limit int64) (Clip, error) {
		t.Helper()
		return Clipboard{}.readClip(t.Context(), limit, pb.name)
	}
	// Text is never a file.
	for _, text := range []string{a, "file://" + a, "hello world"} {
		pb.set("text", text)
		if c, err := read(1 << 20); !errors.Is(err, ErrClipEmpty) {
			t.Errorf("text %q: %+v, %v", text, c, err)
		}
	}
	// A copied file, its icon beside it, and one by its file reference URL.
	for _, kind := range []string{"files", "ref"} {
		pb.set(kind, b)
		if c, err := read(1 << 20); err != nil || c.File != b || c.Image != nil {
			t.Errorf("%s: %+v, %v", kind, c, err)
		}
	}
	// Several files.
	pb.set("files", a, b)
	var many *ClipManyFiles
	if c, err := read(1 << 20); !errors.As(err, &many) || many.N != 2 {
		t.Errorf("two files: %+v, %v", c, err)
	}
	// A copied file that's gone: a failure, never its icon.
	pb.set("ref", gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if c, err := read(1 << 20); !errors.Is(err, ErrClipFileGone) {
		t.Errorf("a file that's gone: %+v, %v", c, err)
	}
	// Images.
	pb.set("image", "public.png", pngFile)
	if c, err := read(1 << 20); err != nil || !bytes.Equal(c.Image, img.Bytes()) || c.Type != "image/png" {
		t.Errorf("a PNG: %+v, %v", c.Type, err)
	}
	pb.set("tiff", pngFile)
	if c, err := read(1 << 20); err != nil || c.Type != "image/png" || len(c.Image) == 0 {
		t.Errorf("a TIFF: %+v, %v", c.Type, err)
	}
	pb.set("textimage", a, pngFile)
	if c, err := read(1 << 20); err != nil || c.Type != "image/png" || c.File != "" {
		t.Errorf("text and an image: %+v, %v", c, err)
	}
	pb.set("image", "public.png", pngFile)
	if c, err := read(int64(img.Len()) - 1); !errors.Is(err, ErrClipTooLarge) {
		t.Errorf("past the cap: %+v, %v", c.Type, err)
	}
	pb.set("clear")
	if c, err := read(1 << 20); !errors.Is(err, ErrClipEmpty) {
		t.Errorf("nothing: %+v, %v", c, err)
	}
}
