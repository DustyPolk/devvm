package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// builtRecent is how long a new tool image is left alone: the VM it was
// built for may not list it in its state yet.
const builtRecent = time.Hour

// NoVM returns the tool images of a List that are current but that no
// VM's state lists, of these names (the names images have them by, every
// name when there are none). One built in the last hour stays, as a VM
// being made may not list it yet. Callers deleting them hold the share's
// lock from before the List, as for Unused.
func (t Share) NoVM(ls []Listed, names []string) []Listed {
	var out []Listed
	for _, l := range ls {
		if l.State != StateCurrent || l.VMs > 0 || l.File == "" || (len(names) > 0 && !slices.Contains(names, l.Name)) {
			continue
		}
		fi, err := os.Stat(filepath.Join(t.Dirs.Tools(), l.File+".sha256"))
		if err != nil || t.Now().Sub(fi.ModTime()) < builtRecent {
			continue
		}
		out = append(out, l)
	}
	return out
}

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
