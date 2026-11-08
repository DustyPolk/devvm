package clipboard

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/devvmhq/devvm/internal/platform"
)

// fakeClipboard stands in for this machine's clipboard: raw is what its
// reader gives, err its error; reads counts the reads.
type fakeClipboard struct {
	raw   []byte
	err   error
	reads int
}

func (f *fakeClipboard) read(context.Context) ([]byte, error) {
	f.reads++
	return f.raw, f.err
}

// newServer is a Server over f, its clock at *now, recording its events.
func newServer(f *fakeClipboard, now *time.Time, events *[]string) *Server {
	return &Server{
		Read:        f.read,
		Event:       func(event, why string) { *events = append(*events, event+": "+why) },
		Now:         func() time.Time { return *now },
		RequestWait: 100 * time.Millisecond,
	}
}

// ask sends req to s over a pipe, in writes as given, and returns what s
// answered before closing it.
func ask(ctx context.Context, t *testing.T, s *Server, req ...string) string {
	t.Helper()
	vm, host := net.Pipe()
	done := make(chan struct{})
	go func() {
		s.Serve(ctx, host)
		close(done)
	}()
	go func() {
		for _, r := range req {
			if _, err := io.WriteString(vm, r); err != nil {
				return // refused before all of it was read
			}
		}
	}()
	answer, _ := io.ReadAll(vm)
	<-done
	_ = vm.Close()
	return string(answer)
}

func TestServe(t *testing.T) {
	good := pngOf(t, 3, 2)
	sanitized, err := Sanitize(good)
	if err != nil {
		t.Fatal(err)
	}
	served := EventServed + ": " + strconv.Itoa(len(sanitized)) + " bytes"
	for _, tt := range []struct {
		name   string
		f      fakeClipboard
		req    []string
		answer string
		events []string
		reads  int
	}{
		{name: "types, an image", f: fakeClipboard{raw: good}, req: []string{"types\n"}, answer: "image/png\n", reads: 1},
		{name: "types, in pieces", f: fakeClipboard{raw: good}, req: []string{"ty", "pes", "\n"}, answer: "image/png\n", reads: 1},
		{name: "types, no image", f: fakeClipboard{err: platform.ErrNoClipboardImage}, req: []string{"types\n"}, reads: 1},
		{
			name: "types, no reader", f: fakeClipboard{err: platform.ErrNoClipboardReader}, req: []string{"types\n"},
			events: []string{"clipboard_refused: no reader"}, reads: 1,
		},
		{
			name: "types, text", f: fakeClipboard{raw: []byte("a password")}, req: []string{"types\n"},
			events: []string{"clipboard_refused: not an image"}, reads: 1,
		},
		{name: "image", f: fakeClipboard{raw: good}, req: []string{"image/png\n"}, answer: string(sanitized), events: []string{served}, reads: 1},
		{
			name: "image, no image", f: fakeClipboard{err: platform.ErrNoClipboardImage}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: no image"}, reads: 1,
		},
		{
			name: "image, over the reader's cap", f: fakeClipboard{err: platform.ErrClipboardTooLarge}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: too large"}, reads: 1,
		},
		{
			name: "image, a decode bomb", f: fakeClipboard{raw: bombPNG(t, 100000, 100000)}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: too large"}, reads: 1,
		},
		{
			name: "image, text", f: fakeClipboard{raw: []byte("a password")}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: not an image"}, reads: 1,
		},
		{
			name: "image, no reader", f: fakeClipboard{err: platform.ErrNoClipboardReader}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: no reader"}, reads: 1,
		},
		{
			name: "image, the reader failed", f: fakeClipboard{err: errors.New("xclip: exit status 2")}, req: []string{"image/png\n"},
			events: []string{"clipboard_refused: reader failed"}, reads: 1,
		},
	} {
		now := time.Unix(1e9, 0)
		var events []string
		s := newServer(&tt.f, &now, &events)
		if got := ask(t.Context(), t, s, tt.req...); got != tt.answer {
			t.Errorf("%s: answered %d bytes %.20q, want %d bytes %.20q", tt.name, len(got), got, len(tt.answer), tt.answer)
		}
		if !slices.Equal(events, tt.events) {
			t.Errorf("%s: events %q, want %q", tt.name, events, tt.events)
		}
		if tt.f.reads != tt.reads {
			t.Errorf("%s: %d reads, want %d", tt.name, tt.f.reads, tt.reads)
		}
	}
}

// Anything but one of the two requests is refused with nothing, before
// the clipboard is read.
func TestServeBadRequest(t *testing.T) {
	for name, req := range map[string][]string{
		"another request":             {"get image/png\n"},
		"another type":                {"image/jpeg\n"},
		"uppercase":                   {"TYPES\n"},
		"a carriage return":           {"types\r\n"},
		"more after the line":         {"types\nimage/png\n"},
		"a 65-byte line":              {strings.Repeat("a", 64) + "\n"},
		"64 bytes, then a newline":    {strings.Repeat("a", 64), "\n"},
		"nothing":                     nil,
		"no newline before it's late": {"image/png"},
	} {
		f := fakeClipboard{raw: pngOf(t, 1, 1)}
		now := time.Unix(1e9, 0)
		var events []string
		s := newServer(&f, &now, &events)
		if answer := ask(t.Context(), t, s, req...); answer != "" || f.reads != 0 {
			t.Errorf("%s: answered %q after %d reads, want nothing and none", name, answer, f.reads)
		}
		if !slices.Equal(events, []string{"clipboard_refused: bad request"}) {
			t.Errorf("%s: events %q", name, events)
		}
	}
}

// A paste asks for the types, then the image: one read, the same image.
// A request after Fresh reads again.
func TestServeFresh(t *testing.T) {
	first, second := pngOf(t, 2, 2), pngOf(t, 9, 9)
	want, _ := Sanitize(first)
	f := fakeClipboard{raw: first}
	now := time.Unix(1e9, 0)
	var events []string
	s := newServer(&f, &now, &events)
	if got := ask(t.Context(), t, s, "types\n"); got != "image/png\n" {
		t.Fatalf("types: %q", got)
	}
	f.raw = second
	now = now.Add(Fresh - time.Millisecond)
	if got := ask(t.Context(), t, s, "image/png\n"); got != string(want) || f.reads != 1 {
		t.Errorf("within Fresh: %d bytes after %d reads, want the first image's %d after 1", len(got), f.reads, len(want))
	}
	now = now.Add(time.Millisecond)
	want, _ = Sanitize(second)
	if got := ask(t.Context(), t, s, "image/png\n"); got != string(want) || f.reads != 2 {
		t.Errorf("after Fresh: %d bytes after %d reads, want the second image's %d after 2", len(got), f.reads, len(want))
	}
	// A failed read is kept as long.
	f.err = platform.ErrNoClipboardReader
	now = now.Add(Fresh)
	ask(t.Context(), t, s, "types\n")
	f.err = nil
	if got := ask(t.Context(), t, s, "image/png\n"); got != "" || f.reads != 3 {
		t.Errorf("within Fresh of a failed read: %d bytes after %d reads, want none after 3", len(got), f.reads)
	}
}

// Requests while a read runs wait for it, and get what it read.
func TestServeOneReadAtATime(t *testing.T) {
	f := fakeClipboard{raw: pngOf(t, 1, 1)}
	now := time.Unix(1e9, 0)
	var events []string
	s := newServer(&f, &now, &events)
	reading, release := make(chan struct{}), make(chan struct{})
	s.Read = func(ctx context.Context) ([]byte, error) {
		close(reading)
		<-release
		return f.read(ctx)
	}
	answers := make(chan string, 2)
	go func() { answers <- ask(t.Context(), t, s, "types\n") }()
	<-reading
	go func() { answers <- ask(t.Context(), t, s, "types\n") }()
	time.Sleep(20 * time.Millisecond) // the second waits on the first's read
	close(release)
	for range 2 {
		if got := <-answers; got != "image/png\n" {
			t.Errorf("answered %q", got)
		}
	}
	if f.reads != 1 {
		t.Errorf("%d reads, want 1", f.reads)
	}
}

// A Server stopping (its context done) closes its connections, and isn't
// a VM's bad request.
func TestServeStopped(t *testing.T) {
	f := fakeClipboard{raw: pngOf(t, 1, 1)}
	now := time.Unix(1e9, 0)
	var events []string
	s := newServer(&f, &now, &events)
	s.RequestWait = time.Hour
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(20*time.Millisecond, cancel)
	if got := ask(ctx, t, s); got != "" || f.reads != 0 || len(events) != 0 {
		t.Errorf("answered %q after %d reads, events %q; want nothing", got, f.reads, events)
	}
}
