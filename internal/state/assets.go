package state

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/paths"
)

// assetsTmp starts the name of a folder WriteAssets is writing.
const assetsTmp = ".tmp-"

// assetsTmpAge is how old a folder WriteAssets was writing is before
// StaleAssets lists it: one a killed run left.
const assetsTmpAge = time.Hour

// AssetsHash names a version of devvm's own files: 16 hex digits of a
// SHA-256 over each regular file's path and contents, in path order.
func AssetsHash(fsys fs.FS) (string, error) {
	h := sha256.New()
	field := func(b []byte) {
		_ = binary.Write(h, binary.BigEndian, uint64(len(b)))
		_, _ = h.Write(b)
	}
	err := fs.WalkDir(fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if err != nil || !e.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		field([]byte(name))
		field(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// WriteAssets writes devvm's own files, fsys, out for what takes a path,
// and returns their folder: d.Assets()/<AssetsHash>. One already there is
// used as is. A new one is written whole beside it, then renamed into
// place, so a run sees a complete folder or none, and runs writing it at
// once all use the first one in. Files are 0600, folders 0700.
func WriteAssets(d paths.Dirs, fsys fs.FS) (string, error) {
	hash, err := AssetsHash(fsys)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(d.Assets(), hash)
	if isDir(dir) {
		return dir, nil
	}
	if err := os.MkdirAll(d.Assets(), 0o700); err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp(d.Assets(), assetsTmp)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // gone once renamed
	err = fs.WalkDir(fsys, ".", func(name string, e fs.DirEntry, err error) error {
		switch {
		case err != nil || name == ".":
			return err
		case e.IsDir():
			return os.Mkdir(filepath.Join(tmp, name), 0o700)
		case !e.Type().IsRegular():
			return nil
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(tmp, name), data, 0o600)
	})
	if err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		if isDir(dir) {
			return dir, nil // another run's, just in
		}
		return "", err
	}
	return dir, nil
}

func isDir(path string) bool {
	fi, err := os.Stat(path) //nolint:gosec // paths come from paths.Dirs
	return err == nil && fi.IsDir()
}

// StaleAssets returns the folders in d.Assets() other than current's (an
// AssetsHash) and those keep names: older versions of devvm's files, and
// what a killed run left writing one an hour or more ago.
func StaleAssets(d paths.Dirs, current string, keep map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(d.Assets())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, e := range entries {
		n := e.Name()
		if n == current || keep[n] {
			continue
		}
		if strings.HasPrefix(n, assetsTmp) {
			fi, err := e.Info()
			if err != nil || time.Since(fi.ModTime()) < assetsTmpAge {
				continue
			}
		}
		stale = append(stale, filepath.Join(d.Assets(), n))
	}
	return stale, nil
}
