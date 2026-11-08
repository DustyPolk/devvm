package clipboard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/devvmhq/devvm/internal/platform"
)

// The events a Server reports, for the VM's event log. A refusal's why
// names what went wrong, never what the clipboard holds.
const (
	EventServed  = "clipboard"         // an image sent, its size as why
	EventRefused = "clipboard_refused" // nothing sent, and why
)

// Fresh is how long a Server keeps a read's result. An agent asks for the
// clipboard's types, then for its image, within one paste: both get one
// read of this machine's clipboard, and the same image.
const Fresh = 3 * time.Second

// A VM's requests, each one line, and the answer to TypesRequest when the
// clipboard holds an image. Anything else is refused.
const (
	TypesRequest = "types\n"
	ImageRequest = "image/png\n"
	TypesAnswer  = "image/png\n"
)

const (
	maxRequest  = 64              // a request's bytes, its newline included
	requestWait = 2 * time.Second // a Server's RequestWait when it has none
	writeWait   = 30 * time.Second
)

// Server answers a VM's requests for the image on this machine's
// clipboard (Serve). Its fields are its seams.
type Server struct {
	// Read reads the clipboard's image, as platform.ClipboardImage gives
	// it: one of its errors when there's none to read.
	Read func(ctx context.Context) ([]byte, error)
	// Event reports what came of a request: EventServed or EventRefused,
	// and why.
	Event func(event, why string)
	// Now is the clock a kept result goes by: time.Now when nil.
	Now func() time.Time
	// RequestWait is how long a VM has to send its request: 2 s when 0.
	RequestWait time.Duration

	mu   sync.Mutex // held through a read, so there's one at a time
	last *result
}

// result is what came of a read: the sanitized PNG, or why there's none.
type result struct {
	at  time.Time
	png []byte
	err error
}

// Serve answers conn's one request, then closes it. TypesRequest gets
// TypesAnswer when the clipboard holds an image, ImageRequest the image as
// a PNG (Sanitize), and either gets nothing otherwise; anything that isn't
// one of them within RequestWait is refused with nothing.
func (s *Server) Serve(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	defer context.AfterFunc(ctx, func() { _ = conn.Close() })()
	req := s.request(conn)
	if req == "" {
		if ctx.Err() == nil {
			s.Event(EventRefused, "bad request")
		}
		return
	}
	img, err := s.image(ctx)
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	switch {
	case err == nil && req == TypesRequest:
		_, _ = io.WriteString(conn, TypesAnswer)
	case err == nil:
		// Reported before it's sent, so a send cut short is still logged.
		s.Event(EventServed, strconv.Itoa(len(img))+" bytes")
		_, _ = conn.Write(img)
	case req == ImageRequest || !errors.Is(err, platform.ErrNoClipboardImage):
		// An agent asks for the types first and stops at none, so a failed
		// read is reported there too; an empty clipboard only when asked
		// for its image, as anything in the VM can ask for its types.
		s.Event(EventRefused, why(err))
	}
}

// request reads conn's request: TypesRequest or ImageRequest, else "".
func (s *Server) request(conn net.Conn) string {
	wait := s.RequestWait
	if wait == 0 {
		wait = requestWait
	}
	if conn.SetReadDeadline(time.Now().Add(wait)) != nil {
		return ""
	}
	buf := make([]byte, maxRequest)
	n := 0
	for n < len(buf) {
		m, err := conn.Read(buf[n:])
		n += m
		if bytes.IndexByte(buf[:n], '\n') >= 0 {
			if req := string(buf[:n]); req == TypesRequest || req == ImageRequest {
				return req
			}
			return ""
		}
		if err != nil {
			return ""
		}
	}
	return ""
}

// image reads the clipboard's image and sanitizes it, or returns the
// result of a read less than Fresh ago.
func (s *Server) image(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last != nil && s.now().Sub(s.last.at) < Fresh {
		return s.last.png, s.last.err
	}
	raw, err := s.Read(ctx)
	var img []byte
	if err == nil {
		img, err = Sanitize(raw)
	}
	s.last = &result{at: s.now(), png: img, err: err}
	return img, err
}

func (s *Server) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// why is a refusal's why for a read's error.
func why(err error) string {
	switch {
	case errors.Is(err, platform.ErrNoClipboardImage):
		return "no image"
	case errors.Is(err, ErrTooLarge), errors.Is(err, platform.ErrClipboardTooLarge):
		return "too large"
	case errors.Is(err, ErrNotImage):
		return "not an image"
	case errors.Is(err, platform.ErrNoClipboardReader):
		return "no reader"
	}
	return "reader failed"
}
