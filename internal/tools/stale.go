package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// miseRecent is how long a folder of devvm's own mise is left alone after
// it changed: another devvm may be downloading into it.
const miseRecent = time.Hour

// StaleMise returns the folders of devvm's own mise (Dirs.Mise) but this
// devvm's version: an older devvm's, which an upgrade leaves. mise's data
// and cache never are there (miseCmd gives each run temporary ones). A
// folder that changed in the last hour stays, as another devvm may be
// downloading its mise into it.
func (t Versions) StaleMise() ([]string, error) {
	entries, err := os.ReadDir(t.Dirs.Mise())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == miseVersion {
			continue
		}
		fi, err := e.Info()
		if err != nil || t.Now().Sub(fi.ModTime()) < miseRecent {
			continue
		}
		out = append(out, filepath.Join(t.Dirs.Mise(), e.Name()))
	}
	return out, nil
}
