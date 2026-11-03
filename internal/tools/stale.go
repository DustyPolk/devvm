package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/w3cj/devvm/internal/state"
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
	out, _ := t.NoVMRecent(ls, names)
	return out
}

// NoVMRecent is NoVM, also returning the ones it keeps because they were
// built in the last hour: devvm gc --tool-images says so, and with --force
// deletes them too.
func (t Share) NoVMRecent(ls []Listed, names []string) (old, recent []Listed) {
	for _, l := range ls {
		if l.State != StateCurrent || l.VMs > 0 || l.File == "" || (len(names) > 0 && !slices.Contains(names, l.Name)) {
			continue
		}
		fi, err := os.Stat(filepath.Join(t.Dirs.Tools(), l.File+".sha256"))
		switch {
		case err != nil:
		case t.Now().Sub(fi.ModTime()) < builtRecent:
			recent = append(recent, l)
		default:
			old = append(old, l)
		}
	}
	return old, recent
}

// idleExt is the file beside a current tool image no VM has, made when
// retention first finds it so: how long it's been idle.
const idleExt = ".idle"

// Idle keeps the record of how long each current tool image of a List
// has had no VM, and returns those idle for age or longer (by now), as
// NoVM picks them: a VM listing it again drops its record. Callers
// deleting them hold the share's lock from before the List.
func (t Share) Idle(ls []Listed, age time.Duration, now time.Time) ([]Listed, error) {
	noVM := map[string]bool{}
	for _, l := range t.NoVM(ls, nil) {
		noVM[l.File] = true
	}
	var out []Listed
	for _, l := range ls {
		if !l.Built || l.File == "" {
			continue
		}
		p := filepath.Join(t.Dirs.Tools(), l.File+idleExt)
		if !noVM[l.File] {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			continue
		}
		fi, err := os.Stat(p)
		if errors.Is(err, fs.ErrNotExist) {
			if err := state.WriteFile(p, nil); err != nil {
				return nil, err
			}
			continue
		} else if err != nil {
			return nil, err
		}
		if since := now.Sub(fi.ModTime()); since >= age && since >= 0 {
			out = append(out, l)
		}
	}
	return out, nil
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
