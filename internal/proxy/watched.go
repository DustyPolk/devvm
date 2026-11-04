package proxy

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"time"
)

// watchTick is the coarsest mtime a host's filesystem keeps (2 s on some, 1 s on Linux's older
// ones): a file whose mtime is within it of now may change again without its stamp changing, so
// it's read again at each call until its mtime is older.
const watchTick = 2 * time.Second

// watched is a file the proxy reads again when it changes, its contents parsed into a T: the
// bindings, the policy files and the links list. Each call stats it (Lstat, or Stat with follow),
// and reads it when its stamp (mtime, size and inode) differs from the last read's, or the
// last read's mtime was within watchTick of then; it parses what it read only when the bytes
// differ. A file that's gone has none, silently. A file that can't be stat'd, read or parsed is
// logged once for each distinct error (onFail) and, with keep, keeps its last good T; without,
// it has none until it's read again.
type watched[T any] struct {
	path   string
	follow bool                              // Stat, for a file read through a symbolic link (ReadRegular)
	keep   bool                              // on a failure, keep the last good T rather than none
	read   func(path string) ([]byte, error) // readPrivate or ReadRegular, with the file's cap
	parse  func(b []byte) (T, error)
	onFail func(err error) // called once for each distinct failure in a row
	// changed, when set, runs with mu held whenever the file changes: its stamp or its bytes (a
	// touch counts), or its T is dropped. A lock it takes comes after mu.
	changed func()

	mu      sync.Mutex
	st      stamp
	settled bool   // st's mtime was older than watchTick when it was read
	raw     []byte // what the last good read gave; nil: none
	val     T
	failed  string // the last failure logged, until a read succeeds or the file goes
}

// stamp is what tells a file's versions apart without reading it.
type stamp struct {
	mtime time.Time
	size  int64
	ino   uint64
}

func stampOf(fi fs.FileInfo) stamp {
	s := stamp{mtime: fi.ModTime(), size: fi.Size()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		s.ino = st.Ino
	}
	return s
}

// get returns the file's T as it is now.
func (w *watched[T]) get() T {
	w.mu.Lock()
	defer w.mu.Unlock()
	stat := os.Lstat
	if w.follow {
		stat = os.Stat
	}
	fi, err := stat(w.path)
	if errors.Is(err, fs.ErrNotExist) {
		w.drop()
		w.failed = ""
		return w.val
	}
	if err != nil {
		return w.fail(err)
	}
	now := stampOf(fi)
	if now == w.st && w.settled {
		return w.val
	}
	// The stamp is taken before the read: a change after it makes the next call's stamp differ.
	b, err := w.read(w.path)
	if err != nil {
		return w.fail(err)
	}
	w.failed = ""
	touched := now != w.st
	w.st, w.settled = now, time.Since(now.mtime) >= watchTick
	if w.raw != nil && bytes.Equal(b, w.raw) {
		if touched && w.changed != nil { // the same bytes, written again or touched
			w.changed()
		}
		return w.val
	}
	v, err := w.parse(b)
	if err != nil {
		return w.fail(err)
	}
	w.raw, w.val = b, v
	if w.changed != nil {
		w.changed()
	}
	return w.val
}

// fail logs err unless it's the failure already logged, and returns the last good T with keep,
// or none. The stamp is forgotten, so the next call tries again.
func (w *watched[T]) fail(err error) T {
	if why := err.Error(); why != w.failed {
		w.failed = why
		w.onFail(err)
	}
	w.st = stamp{}
	if !w.keep {
		w.drop()
	}
	return w.val
}

// drop forgets the T, running changed if there was one.
func (w *watched[T]) drop() {
	w.st = stamp{}
	if w.raw == nil {
		return
	}
	var zero T
	w.raw, w.val = nil, zero
	if w.changed != nil {
		w.changed()
	}
}
