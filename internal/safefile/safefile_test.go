//go:build unix

package safefile

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// within runs f, failing the test when it takes more than a few seconds: a read that waits on a
// FIFO never returns.
func within(t *testing.T, name string, f func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waits", name)
		return nil
	}
}

// folder makes a folder of every kind of name a VM can leave: a file, links to it and out of the
// folder, a FIFO, a link to a device, a folder, a file past the cap, and a link through a folder.
func folder(t *testing.T) (dir, outside string) {
	t.Helper()
	dir, outside = t.TempDir(), filepath.Join(t.TempDir(), "outside")
	must(t, os.WriteFile(outside, []byte("outside\n"), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "file"), []byte("file\n"), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "big"), bytes.Repeat([]byte("x"), 65), 0o600))
	must(t, os.Symlink("file", filepath.Join(dir, "link")))
	must(t, os.Symlink(outside, filepath.Join(dir, "out")))
	must(t, os.Symlink("/dev/zero", filepath.Join(dir, "zero")))
	must(t, syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o600))
	must(t, os.Mkdir(filepath.Join(dir, "sub"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "file"), []byte("sub\n"), 0o600))
	must(t, os.Symlink("sub", filepath.Join(dir, "sublink")))
	must(t, os.Symlink(filepath.Dir(outside), filepath.Join(dir, "outdir")))
	return dir, outside
}

// Read, and a Root's, reads a regular file within its cap; refuses a FIFO, a device, a folder
// and a file past the cap, each with its error, without waiting; and follows a link at the end
// only with Follow, beneath a Root only to a file in it.
func TestRead(t *testing.T) {
	t.Parallel()
	dir, _ := folder(t)
	root, err := OpenRoot(dir)
	must(t, err)
	t.Cleanup(func() { _ = root.Close() })
	const limit = 64
	for _, c := range []struct {
		name   string
		o      Opts
		want   string // the file's text, or "" for an error
		err    error  // the error it wraps, when it's one of this package's
		inRoot string // what the Root gives, when it differs: "!" for an error
	}{
		{name: "file", want: "file\n"},
		{name: "big", err: ErrTooLong},
		{name: "fifo", err: ErrNotRegular},
		{name: "fifo", o: Opts{Follow: true}, err: ErrNotRegular},
		{name: "sub", err: ErrNotRegular},
		{name: "link", err: ErrNotRegular},
		{name: "link", o: Opts{Follow: true}, want: "file\n"},
		{name: "out", err: ErrNotRegular},
		{name: "out", o: Opts{Follow: true}, want: "outside\n", inRoot: "!"},
		{name: "zero", err: ErrNotRegular},
		{name: "zero", o: Opts{Follow: true}, err: ErrNotRegular, inRoot: "!"},
		// A link on the way: followed, beneath a Root only to a folder in it.
		{name: "sublink/file", want: "sub\n"},
		{name: "outdir/outside", want: "outside\n", inRoot: "!"},
		{name: "fifo/file", inRoot: "!"},
		{name: "missing", err: fs.ErrNotExist},
	} {
		check := func(how string, got []byte, err error, want string, wantErr error) {
			t.Helper()
			switch {
			case want != "" && (err != nil || string(got) != want):
				t.Errorf("%s(%s, %+v) = %q, %v, want %q", how, c.name, c.o, got, err, want)
			case want == "" && err == nil:
				t.Errorf("%s(%s, %+v) = %q, want an error", how, c.name, c.o, got)
			case wantErr != nil && !errors.Is(err, wantErr):
				t.Errorf("%s(%s, %+v) = %v, want %v", how, c.name, c.o, err, wantErr)
			}
		}
		var got []byte
		err := within(t, c.name, func() (err error) { got, err = Read(filepath.Join(dir, c.name), limit, c.o); return err })
		check("Read", got, err, c.want, c.err)
		want, wantErr := c.want, c.err
		if c.inRoot == "!" {
			want, wantErr = "", nil
		}
		err = within(t, c.name, func() (err error) { got, err = root.Read(c.name, limit, c.o); return err })
		check("Root.Read", got, err, want, wantErr)
	}
}

// An error names the file it's about.
func TestReadErrorNamesFile(t *testing.T) {
	t.Parallel()
	dir, _ := folder(t)
	root, err := OpenRoot(dir)
	must(t, err)
	defer func() { _ = root.Close() }()
	for _, name := range []string{"fifo", "big", "link"} {
		p := filepath.Join(dir, name)
		if _, err := Read(p, 64, Opts{}); err == nil || !bytes.Contains([]byte(err.Error()), []byte(p)) {
			t.Errorf("Read(%s) = %v", name, err)
		}
		if _, err := root.Read(name, 64, Opts{}); err == nil || !bytes.Contains([]byte(err.Error()), []byte(p)) {
			t.Errorf("Root.Read(%s) = %v", name, err)
		}
	}
}

// A file at its cap is read whole; one that grows past it once open is refused.
func TestReadAllCap(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	must(t, os.WriteFile(p, []byte("12345"), 0o600))
	if b, err := Read(p, 5, Opts{}); err != nil || string(b) != "12345" {
		t.Errorf("at its cap: %q, %v", b, err)
	}
	f, err := Open(p, Opts{})
	must(t, err)
	defer func() { _ = f.Close() }()
	w, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0) //nolint:gosec // the test's own file
	must(t, err)
	_, err = w.WriteString("678")
	must(t, errors.Join(err, w.Close()))
	if b, err := f.ReadAll(6); !errors.Is(err, ErrTooLong) {
		t.Errorf("grown past its cap: %q, %v", b, err)
	}
}

// OneLink refuses a file with another hard link, which is read without it.
func TestOneLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p, other := filepath.Join(dir, "f"), filepath.Join(dir, "g")
	must(t, os.WriteFile(p, []byte("x"), 0o600))
	if _, err := Read(p, 1, Opts{OneLink: true}); err != nil {
		t.Errorf("one link: %v", err)
	}
	must(t, os.Link(p, other))
	if _, err := Read(p, 1, Opts{OneLink: true}); !errors.Is(err, ErrLinked) {
		t.Errorf("two links: %v, want ErrLinked", err)
	}
	if b, err := Read(p, 1, Opts{}); err != nil || string(b) != "x" {
		t.Errorf("two links without OneLink: %q, %v", b, err)
	}
}

// A file swapped for a FIFO, a link out or back as it's read never holds the reader: each read
// gives the file or an error.
func TestSwapDuringOpen(t *testing.T) {
	t.Parallel()
	dir, spare := t.TempDir(), t.TempDir()
	target := filepath.Join(dir, "f")
	stop, swapped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(swapped)
		file, fifo, link := filepath.Join(spare, "file"), filepath.Join(spare, "fifo"), filepath.Join(spare, "link")
		for {
			select {
			case <-stop:
				return
			default:
			}
			if os.WriteFile(file, []byte("file\n"), 0o600) != nil || os.Rename(file, target) != nil ||
				syscall.Mkfifo(fifo, 0o600) != nil || os.Rename(fifo, target) != nil ||
				os.Symlink("/dev/zero", link) != nil || os.Rename(link, target) != nil {
				return
			}
		}
	}()
	defer func() { close(stop); <-swapped }()
	root, err := OpenRoot(dir)
	must(t, err)
	defer func() { _ = root.Close() }()
	reads := map[string]func() ([]byte, error){
		"Read":              func() ([]byte, error) { return Read(target, 64, Opts{}) },
		"Read, Follow":      func() ([]byte, error) { return Read(target, 64, Opts{Follow: true}) },
		"Root.Read":         func() ([]byte, error) { return root.Read("f", 64, Opts{}) },
		"Root.Read, Follow": func() ([]byte, error) { return root.Read("f", 64, Opts{Follow: true}) },
		"Root.FS's Open":    func() ([]byte, error) { return fs.ReadFile(root.FS(), "f") },
		"Open, then read":   func() ([]byte, error) { f, err := Open(target, Opts{}); return closeRead(f, err) },
	}
	for name, read := range reads {
		_ = within(t, name, func() error {
			for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); {
				if b, err := read(); err == nil && string(b) != "file\n" {
					return errors.New(string(b))
				}
			}
			return nil
		})
	}
}

func closeRead(f *File, err error) ([]byte, error) {
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadAll(64)
}

// A Root's FS opens only regular files, and lists and stats what's there without opening it.
func TestRootFS(t *testing.T) {
	t.Parallel()
	dir, _ := folder(t)
	root, err := OpenRoot(dir)
	must(t, err)
	defer func() { _ = root.Close() }()
	fsys := root.FS()
	if b, err := fs.ReadFile(fsys, "sub/file"); err != nil || string(b) != "sub\n" {
		t.Errorf("ReadFile(sub/file) = %q, %v", b, err)
	}
	if b, err := fs.ReadFile(fsys, "link"); err != nil || string(b) != "file\n" {
		t.Errorf("ReadFile(link) = %q, %v", b, err)
	}
	for _, name := range []string{"fifo", "sub", "."} {
		if err := within(t, name, func() error { _, err := fsys.Open(name); return err }); !errors.Is(err, ErrNotRegular) {
			t.Errorf("Open(%s) = %v, want ErrNotRegular", name, err)
		}
	}
	for _, name := range []string{"out", "/etc/passwd", "../x"} {
		if _, err := fsys.Open(name); err == nil {
			t.Errorf("Open(%s) opened", name)
		}
	}
	if fi, err := fs.Stat(fsys, "fifo"); err != nil || fi.Mode()&fs.ModeNamedPipe == 0 {
		t.Errorf("Stat(fifo) = %v, %v", fi, err)
	}
	var names []string
	must(t, fs.WalkDir(fsys, ".", func(p string, _ fs.DirEntry, err error) error {
		names = append(names, p)
		return err
	}))
	if want := []string{".", "big", "fifo", "file", "link", "out", "outdir", "sub", "sub/file", "sublink", "zero"}; !slices.Equal(names, want) {
		t.Errorf("WalkDir = %v, want %v", names, want)
	}
}
