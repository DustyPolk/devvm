// Package fetch is how devvm reaches the web: an HTTP client with a
// timeout and a redirect policy, retries of the failures that may pass,
// reads capped at a size, and downloads checked against their SHA-256.
// It is the standard library only.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is the HTTP client devvm uses, made by NewClient: other packages
// name this type, not net/http's.
type Client = http.Client

// Redirects is the redirects a client follows.
type Redirects int

const (
	// HTTPSOnly follows redirects to https addresses only.
	HTTPSOnly Redirects = iota
	// SameScheme follows redirects on the first request's scheme only, so
	// a server can't send a client that reads file:// addresses to a file.
	SameScheme
)

// maxRedirects is how many redirects a client follows, as net/http's own
// client does.
const maxRedirects = 10

var (
	// ErrRedirect is a redirect refused: trying again won't help.
	ErrRedirect = errors.New("redirect refused")
	// ErrTooLarge is a body over its cap.
	ErrTooLarge = errors.New("too large")
	// ErrChecksum is a download whose SHA-256 isn't the one asked for.
	ErrChecksum = errors.New("the SHA-256 doesn't match")
	// errRequest is a request that can't be made, such as one for an
	// address that isn't one.
	errRequest = errors.New("no request")
)

// NewClient returns a client whose every request, its body included, takes
// at most timeout, following redirects as r says, on its own copy of
// net/http's default transport (environment proxies included), which a
// caller can change.
func NewClient(timeout time.Duration, r Redirects) *Client {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t = &http.Transport{}
	}
	check := httpsOnly
	if r == SameScheme {
		check = sameScheme
	}
	return &http.Client{Transport: t.Clone(), Timeout: timeout, CheckRedirect: check}
}

func httpsOnly(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("%w: too many", ErrRedirect)
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("%w: to %s", ErrRedirect, req.URL.Scheme)
	}
	return nil
}

func sameScheme(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("%w: too many", ErrRedirect)
	}
	if req.URL.Scheme != via[0].URL.Scheme {
		return fmt.Errorf("%w: from %s to %s", ErrRedirect, via[0].URL.Scheme, req.URL.Scheme)
	}
	return nil
}

// StatusError is an answer other than 200 OK.
type StatusError struct {
	URL    string
	Code   int
	Status string // the status line's text, such as "404 Not Found"
}

func (e *StatusError) Error() string { return e.URL + ": " + e.Status }

// TransientStatus reports whether an answer with this status code may be
// different when asked again: 408, 429 or a server error.
func TransientStatus(code int) bool {
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests || code >= 500
}

// Transient reports whether a request that failed with err may succeed
// when tried again: one that got no answer or lost it partway, or got a
// status TransientStatus names. A refused redirect, a request that can't
// be made, a body over its cap and a SHA-256 that doesn't match are not.
func Transient(err error) bool {
	var se *StatusError
	switch {
	case err == nil:
		return false
	case errors.As(err, &se):
		return TransientStatus(se.Code)
	case errors.Is(err, ErrRedirect), errors.Is(err, errRequest), errors.Is(err, ErrTooLarge), errors.Is(err, ErrChecksum):
		return false
	}
	return true
}

// Retry runs try until it succeeds, it fails and says not to try again,
// the context ends, or it has been tried again retries times. It waits
// delay before the first retry and twice as long before each one after.
// It returns try's last error, or the context's once it has ended.
func Retry(ctx context.Context, retries int, delay time.Duration, try func() (again bool, err error)) error {
	for n := 0; ; n++ {
		if err := ctx.Err(); err != nil { // file:// reads don't watch the context
			return err
		}
		again, err := try()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !again || n == retries {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// ReadCapped reads r to its end, failing with ErrTooLarge when it holds
// more than limit bytes.
func ReadCapped(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTooLarge, limit)
	}
	return data, nil
}

// Open GETs url and returns the body of a 200 OK answer, which the caller
// closes; any other answer is a *StatusError.
func Open(ctx context.Context, c *Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errRequest, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, &StatusError{URL: url, Code: resp.StatusCode, Status: resp.Status}
	}
	return resp.Body, nil
}

// Get returns the body of a 200 OK answer to a GET of url, failing with
// ErrTooLarge when it's more than limit bytes.
func Get(ctx context.Context, c *Client, url string, limit int64) ([]byte, error) {
	body, err := Open(ctx, c, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	data, err := ReadCapped(body, limit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", url, err)
	}
	return data, nil
}

// Download writes the body of a GET of url to path, mode 600, and keeps
// it only when its SHA-256 is sum (hex): one that isn't fails with
// ErrChecksum, before anything can use it. On any failure nothing is left
// at path.
func Download(ctx context.Context, c *Client, url, path, sum string) (err error) {
	body, err := Open(ctx, c, url)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the caller's own path
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); sum == "" || !strings.EqualFold(got, sum) {
		return fmt.Errorf("%s: %w: %s", url, ErrChecksum, got)
	}
	return nil
}
