package vm

import (
	"archive/tar"
	"bytes"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/devvmhq/devvm/internal/hostsafety"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/safefile"
)

// The bounds of a workspace clone's copy of the folder's files: each file
// at most wsFileMax bytes, all of them at most wsFilesMax, and at most
// wsFilesCount files, folders and links.
const (
	wsFileMax    = 10 << 20
	wsFilesMax   = 64 << 20
	wsFilesCount = 5000
)

// workspaceFiles is a tar of the files in the folder dir outside every
// repo in it, for a workspace clone's VM: the folder's own files, which
// no repo's clone has, such as a Makefile or a compose file at its top.
// It walks dir one folder at a time, each opened from its parent's
// (safefile.Root.OpenDir) and each file by its own name in it, so no
// link on the way is followed, even one swapped in as it walks, and stays
// on dir's file system. It leaves out every folder with a .git (a repo,
// whether the clone has it or not) or shaped like a git dir (a bare
// repo), every .git, the folders of packages and build tools
// (hostsafety.NoRepos), which the VM fetches for itself, and, counting
// them (skipped), what it won't copy: a link that leads out of dir (it
// keeps one that stays in, as a link), a file with another hard link,
// anything but files, folders and links, a bare repo, a folder that
// changed as it was opened, and what's past its bounds. Files keep their
// permissions but set-id bits. nil when there's nothing to copy.
func workspaceFiles(dir string) (tarball []byte, skipped int, err error) {
	root, err := safefile.OpenRoot(dir)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = root.Close() }()
	top, err := root.Stat()
	if err != nil {
		return nil, 0, err
	}
	w := &filesTar{top: top}
	w.tw = tar.NewWriter(&w.buf)
	if err := w.folder(root, "."); err != nil {
		return nil, 0, err
	}
	if err := w.tw.Close(); err != nil {
		return nil, 0, err
	}
	if w.n == 0 {
		return nil, w.skipped, nil
	}
	return w.buf.Bytes(), w.skipped, nil
}

// filesTar is one workspaceFiles's tar as it's made.
type filesTar struct {
	top     fs.FileInfo
	buf     bytes.Buffer
	tw      *tar.Writer
	n       int   // entries written
	size    int64 // their files' bytes
	skipped int
}

// folder adds what's in the folder d, at rel ("." for the top), each
// entry by its name in d.
func (w *filesTar) folder(d *safefile.Root, rel string) error {
	entries, err := d.ReadDir()
	if err != nil {
		w.skipped++
		return nil //nolint:nilerr // an unreadable folder: left out, and counted
	}
	for _, e := range entries {
		name, p := e.Name(), path.Join(rel, e.Name())
		var err error
		switch {
		case hostsafety.IsGitName(name):
		case w.n >= wsFilesCount:
			w.skipped++
		case e.IsDir():
			err = w.sub(d, name, p)
		case e.Type()&fs.ModeSymlink != 0:
			err = w.link(d, name, p)
		case e.Type().IsRegular():
			err = w.file(d, name, p)
		default:
			w.skipped++
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// sub is folder's for the folder name in d, at p: a package or build
// tool's, a repo's or one shaped like a git dir isn't entered, nor one on
// another file system or that changed as it was opened.
func (w *filesTar) sub(d *safefile.Root, name, p string) error {
	if hostsafety.NoRepos(name) {
		return nil
	}
	s, err := d.OpenDir(name)
	if err != nil {
		w.skipped++
		return nil
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Lstat(".git"); err == nil {
		return nil
	}
	fi, err := s.Stat()
	if err != nil || !platform.SameDevice(w.top, fi) || gitDirShaped(s) {
		w.skipped++
		return nil
	}
	w.n++
	if err := w.tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: p + "/", Mode: int64(fi.Mode().Perm() | 0o700), ModTime: fi.ModTime()}); err != nil {
		return err
	}
	return w.folder(s, p)
}

// gitDirShaped reports whether the folder d has a git dir's HEAD, objects
// and refs, as a bare repo has (hostsafety.IsGitDir's test, by name).
func gitDirShaped(d *safefile.Root) bool {
	for _, n := range []string{"HEAD", "objects", "refs"} {
		if _, err := d.Lstat(n); err != nil {
			return false
		}
	}
	return true
}

// link is folder's for the symbolic link name in d, at p: kept, as a
// link, when it leads to a path in the folder; else left out.
func (w *filesTar) link(d *safefile.Root, name, p string) error {
	target, err := d.Readlink(name)
	if err != nil || filepath.IsAbs(target) || !filepath.IsLocal(filepath.Join(filepath.Dir(p), target)) {
		w.skipped++
		return nil //nolint:nilerr // a link that can't be read: left out, and counted
	}
	w.n++
	return w.tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: p, Linkname: target, Mode: 0o777})
}

// file is folder's for the regular file name in d, at p: never through a
// link, nor one with another hard link, which can be a file's anywhere on
// its disk (safefile's OneLink), within the bounds.
func (w *filesTar) file(d *safefile.Root, name, p string) error {
	f, err := d.Open(name, safefile.Opts{OneLink: true})
	if err != nil {
		w.skipped++
		return nil
	}
	defer func() { _ = f.Close() }()
	size := f.Info.Size()
	if size > wsFileMax || w.size+size > wsFilesMax {
		w.skipped++
		return nil
	}
	b, err := f.ReadAll(wsFileMax)
	if err != nil || w.size+int64(len(b)) > wsFilesMax {
		w.skipped++
		return nil
	}
	w.n++
	w.size += int64(len(b))
	h := &tar.Header{Typeflag: tar.TypeReg, Name: p, Size: int64(len(b)), Mode: int64(f.Info.Mode().Perm()), ModTime: f.Info.ModTime()}
	if err := w.tw.WriteHeader(h); err != nil {
		return err
	}
	_, err = w.tw.Write(b)
	return err
}
