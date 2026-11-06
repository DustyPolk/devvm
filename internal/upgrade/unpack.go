package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// unpackCap caps what an archive unpacks to: devvm's binary and three
// text files are far less.
const unpackCap = 512 << 20

// unpack writes the files of archive, a release's tar.gz, into dir, which
// it makes and which must not exist: the regular files in its one
// top-level folder, top, and nothing else. devvm is mode 755, the rest
// 644. An archive with anything else in it (a link, a folder below top, a
// path out of it) is refused, and so is one without devvm.
func unpack(archive, top, dir string) (err error) {
	f, err := os.Open(archive) //nolint:gosec // the archive this run downloaded and checked
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o755); err != nil { //nolint:gosec // a version's folder, which the user runs devvm from
		return err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	var total int64
	found := false
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name, ok := entryName(h, top)
		if !ok {
			return fmt.Errorf("%s isn't a file of %s", h.Name, top)
		}
		if name == "" {
			continue
		}
		if total += h.Size; total > unpackCap {
			return fmt.Errorf("more than %d bytes", unpackCap)
		}
		if err := writeEntry(tr, filepath.Join(dir, name), name == "devvm"); err != nil {
			return err
		}
		found = found || name == "devvm"
	}
	if !found {
		return errors.New("no devvm in it")
	}
	return nil
}

// entryName is the name of h's file in top, "" for top itself; false for
// anything but top and the regular files right in it.
func entryName(h *tar.Header, top string) (string, bool) {
	name := strings.TrimSuffix(h.Name, "/")
	if name == top || name == "./"+top {
		return "", h.Typeflag == tar.TypeDir
	}
	rest, ok := strings.CutPrefix(strings.TrimPrefix(name, "./"), top+"/")
	if !ok || rest == "" || rest == "." || rest == ".." || strings.ContainsAny(rest, `/\`) {
		return "", false
	}
	return rest, h.Typeflag == tar.TypeReg
}

// writeEntry writes r to path, a new file, executable or not.
func writeEntry(r io.Reader, path string, exe bool) error {
	mode := os.FileMode(0o644)
	if exe {
		mode = 0o755
	}
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) //nolint:gosec // a new file in the folder unpack made
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(r, unpackCap))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
