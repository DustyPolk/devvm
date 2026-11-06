//go:build e2e

package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// NoLeak fails when a regular file under one of paths holds one of
// secrets, byte for byte: a VM's disk, Lima's
// files, devvm's logs. It reads only where a file has data: a VM's disk
// is sparse, 100G of mostly holes, which SEEK_DATA and SEEK_HOLE skip. A
// file it can't read fails it too; a path that isn't there has none, and
// a symbolic link is never followed. The error names the file, never the
// secret.
func NoLeak(secrets []string, paths ...string) error {
	_, err := scanLeaks(secrets, paths...)
	return err
}

// scanLeaks is NoLeak, and how many bytes it read.
func scanLeaks(secrets []string, paths ...string) (int64, error) {
	l := &leaks{}
	for _, s := range secrets {
		if s == "" {
			return 0, errors.New("NoLeak: an empty secret")
		}
		l.secrets = append(l.secrets, []byte(s))
		l.keep = max(l.keep, len(s)-1)
	}
	if len(l.secrets) == 0 {
		return 0, errors.New("NoLeak: no secrets")
	}
	var unread []error
	for _, p := range paths {
		if !Exists(p) {
			continue
		}
		err := filepath.WalkDir(p, func(f string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				unread = append(unread, err)
				return nil
			case !d.Type().IsRegular():
				return nil
			}
			return l.file(f, &unread)
		})
		if err != nil {
			return l.read, err
		}
	}
	return l.read, errors.Join(unread...)
}

// leaks is a NoLeak's: its secrets, how many bytes a chunk keeps of the
// one before it (so a secret across them is found), and the bytes read.
type leaks struct {
	secrets [][]byte
	keep    int
	read    int64
}

// leakChunk is how much a scan reads at once.
const leakChunk = 8 << 20

// file scans f's data extents: an error when one holds a secret, f added
// to unread when it can't be read.
func (l *leaks) file(f string, unread *[]error) error {
	fh, err := os.Open(f)
	if err != nil {
		*unread = append(*unread, err)
		return nil
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		*unread = append(*unread, err)
		return nil
	}
	size := st.Size()
	for at := int64(0); at < size; {
		start, err := fh.Seek(at, unix.SEEK_DATA)
		if errors.Is(err, syscall.ENXIO) {
			return nil // only a hole past at
		}
		end := size
		if err == nil {
			if e, err := fh.Seek(start, unix.SEEK_HOLE); err == nil && e > start {
				end = e
			}
		} else {
			start = at // no SEEK_DATA here: the rest is data
		}
		found, err := l.extent(fh, start, end)
		switch {
		case err != nil:
			*unread = append(*unread, fmt.Errorf("%s: %w", f, err))
			return nil
		case found:
			return fmt.Errorf("a secret is in %s", f)
		}
		at = end
	}
	return nil
}

// extent is whether f's bytes from start to end hold a secret.
func (l *leaks) extent(f *os.File, start, end int64) (bool, error) {
	buf := make([]byte, l.keep+leakChunk)
	tail := 0
	for at := start; at < end; {
		n, err := f.ReadAt(buf[tail:tail+int(min(leakChunk, end-at))], at)
		l.read += int64(n)
		if n > 0 {
			data := buf[:tail+n]
			for _, s := range l.secrets {
				if bytes.Contains(data, s) {
					return true, nil
				}
			}
			tail = min(l.keep, len(data))
			copy(buf, data[len(data)-tail:])
			at += int64(n)
		}
		switch {
		case errors.Is(err, io.EOF):
			return false, nil // the file shrank
		case err != nil:
			return false, err
		}
	}
	return false, nil
}
