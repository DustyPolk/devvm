package msg

import (
	"errors"
	"sync"
)

// IDOf returns the ID of the message err is or wraps, or "" when it
// carries none.
func IDOf(err error) string {
	if m, ok := errors.AsType[M](err); ok {
		return m.ID()
	}
	return ""
}

// NamesOf returns the names of the message err is or wraps (no_vm
// dir=/x), err's own text when it carries none, or "" for nil.
func NamesOf(err error) string {
	if m, ok := errors.AsType[M](err); ok {
		return m.Names()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

// A Recorder keeps the messages its Add is given, in order, for a test
// to check: its Add is a Fake's OnInfo or OnWarn. It's safe
// for concurrent use; its zero value is empty and ready.
type Recorder struct {
	mu sync.Mutex
	ms []M
}

// Add keeps m.
func (r *Recorder) Add(m M) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ms = append(r.ms, m)
}

// Messages returns the messages kept so far.
func (r *Recorder) Messages() []M {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]M(nil), r.ms...)
}

// Names returns each kept message's names, in order: nil for none.
func (r *Recorder) Names() []string {
	return r.each(M.Names)
}

// IDs returns each kept message's ID, in order: nil for none.
func (r *Recorder) IDs() []string {
	return r.each(M.ID)
}

// Reset forgets every message kept so far.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ms = nil
}

func (r *Recorder) each(f func(M) string) []string {
	var out []string
	for _, m := range r.Messages() {
		out = append(out, f(m))
	}
	return out
}
