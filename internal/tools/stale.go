package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/devvmhq/devvm/internal/state"
)

// NoVM returns the tool images of a List that are current but that no
// VM's state lists, of these names (the names images have them by, every
// name when there are none), each with its record. Callers deleting them
// hold the share's lock from before the List, as for Unused, and the
// making lock alone (Making.Exclude): a VM being made may not list the
// tools it picked yet.
func (t Share) NoVM(ls []Listed, names []string) []Listed {
	var out []Listed
	for _, l := range ls {
		if l.State != StateCurrent || l.VMs > 0 || l.File == "" || (len(names) > 0 && !slices.Contains(names, l.Name)) {
			continue
		}
		if _, err := state.ReadToolImage(t.Dirs, l.File); err == nil {
			out = append(out, l)
		}
	}
	return out
}

// Idle keeps, in each tool image's record of a List, since when the
// current ones have had no VM (idle_since, set when retention first finds
// one so), and returns those idle for age or longer (by now), as NoVM
// picks them: a VM listing it again clears it. Callers deleting them hold
// the share's lock from before the List.
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
		rec, err := state.ReadToolImage(t.Dirs, l.File)
		if errors.Is(err, fs.ErrNotExist) {
			continue // a build moving it in: its record comes next
		} else if err != nil {
			return nil, err
		}
		switch {
		case !noVM[l.File] && !rec.IdleSince.IsZero():
			rec.IdleSince = time.Time{}
		case !noVM[l.File]:
			continue
		case rec.IdleSince.IsZero():
			rec.IdleSince = now.UTC().Truncate(time.Second)
		default:
			if since := now.Sub(rec.IdleSince); since >= age && since >= 0 {
				out = append(out, l)
			}
			continue
		}
		if err := state.WriteToolImage(t.Dirs, l.File, rec); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// IdleFor is how long a current tool image of a List has had no VM, as
// Idle recorded it; 0 when it hasn't, or a VM has it. It changes nothing.
func (t Share) IdleFor(l Listed, now time.Time) time.Duration {
	if l.State != StateCurrent || l.VMs > 0 || l.File == "" {
		return 0
	}
	rec, err := state.ReadToolImage(t.Dirs, l.File)
	if err != nil || rec.IdleSince.IsZero() {
		return 0
	}
	return max(now.Sub(rec.IdleSince), 0)
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
