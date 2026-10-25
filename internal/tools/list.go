package tools

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/platform"
)

// The states of a tool image, as devvm images lists them.
const (
	StateCurrent      = "current"       // the lock's version, for base's C library
	StateInUse        = "in use"        // not current, and a VM lists it
	StateUnused       = "unused"        // neither current nor listed by a VM
	StateNotBuilt     = "not built"     // the lock's version, which the share lacks
	StateNotInstalled = "not installed" // nothing of the name's is in the share
)

// Listed is one of the share's tool images as devvm images lists it.
type Listed struct {
	Name    string // the name images have it by: node (for node, pnpm and bun), claude
	Tool    string // "" when nothing of Name's is in the share
	Version string
	Built   bool  // the share has it; Size and VMs are its
	Size    int64 // the disk space it takes
	VMs     int   // how many VMs list it in their state
	// State is one of the State constants; "" for an image the VMs don't
	// list when there's no lock to tell whether it's current.
	State string
}

// ReadLock reads the tools lock as it is, never resolving it: nil when
// there's none yet.
func (t *Tools) ReadLock() (Lock, error) {
	data, err := os.ReadFile(t.Dirs.ToolLock())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return Lock(data), err
}

// List lists the share's tool images by the names images have them by,
// the toolchains then the agents, and each name's tools in order, newest
// version first; a name with none of its tools in the share as not
// installed. inUse counts the VMs that list each image file. The lock is
// read as it is (List never goes online): an image at its version for
// libc is current, and when the share has some of a name's tools but not
// the lock's version of one, that version is listed as not built. Without
// a lock, an image no VM lists has no state.
func (t *Tools) List(libc string, inUse map[string]int) ([]Listed, error) {
	entries, err := os.ReadDir(t.Dirs.Tools())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	lock, err := t.ReadLock()
	if err != nil {
		return nil, err
	}
	files := map[string][]string{} // by tool
	for _, e := range entries {
		if tool, _, ok := image.ParseToolFile(e.Name()); ok && e.Type().IsRegular() {
			files[tool] = append(files[tool], e.Name())
		}
	}
	var out []Listed
	for _, name := range slices.Concat(image.Toolchains, image.Agents) {
		var rows []Listed
		has := false
		for _, tool := range image.GroupTools(name) {
			has = has || len(files[tool]) > 0
		}
		if !has {
			out = append(out, Listed{Name: name, State: StateNotInstalled})
			continue
		}
		for _, tool := range image.GroupTools(name) {
			current := ""
			if lock != nil {
				current, _ = t.File(lock, tool, libc) // a tool the lock lacks has none
			}
			if current != "" && !slices.Contains(files[tool], current) {
				rows = append(rows, Listed{Name: name, Tool: tool, Version: lock.Version(tool), State: StateNotBuilt})
			}
			have := files[tool]
			slices.SortFunc(have, func(a, b string) int {
				_, va, _ := image.ParseToolFile(a)
				_, vb, _ := image.ParseToolFile(b)
				return cmp.Or(compareVersions(vb, va), strings.Compare(a, b))
			})
			for _, f := range have {
				_, v, _ := image.ParseToolFile(f)
				size, err := platform.DiskUsage(filepath.Join(t.Dirs.Tools(), f))
				if err != nil {
					return nil, err
				}
				l := Listed{Name: name, Tool: tool, Version: v, Built: true, Size: size, VMs: inUse[f]}
				switch {
				case f == current:
					l.State = StateCurrent
				case l.VMs > 0:
					l.State = StateInUse
				case lock != nil:
					l.State = StateUnused
				}
				rows = append(rows, l)
			}
		}
		out = append(out, rows...)
	}
	return out, nil
}

// compareVersions compares two versions by their parts, numbers as
// numbers: 1.10.0 is newer than 1.9.2.
func compareVersions(a, b string) int {
	for a != "" && b != "" {
		pa, ra := versionPart(a)
		pb, rb := versionPart(b)
		na, erra := strconv.ParseUint(pa, 10, 64)
		nb, errb := strconv.ParseUint(pb, 10, 64)
		var c int
		if erra == nil && errb == nil {
			c = cmp.Compare(na, nb)
		} else {
			c = strings.Compare(pa, pb)
		}
		if c != 0 {
			return c
		}
		a, b = ra, rb
	}
	return cmp.Compare(len(a), len(b))
}

// versionPart splits a version's first part from the rest: a run of
// digits, or of anything else.
func versionPart(v string) (part, rest string) {
	digit := func(r byte) bool { return r >= '0' && r <= '9' }
	i := 1
	for i < len(v) && digit(v[i]) == digit(v[0]) {
		i++
	}
	return v[:i], v[i:]
}
