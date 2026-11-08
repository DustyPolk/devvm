package gitprotect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/devvmhq/devvm/internal/hostgit"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/safefile"
	"github.com/devvmhq/devvm/internal/state"
)

// On hosts without locks (Linux), devvm can't stop the VM changing what a
// lock would cover (paths), so it watches those paths instead: each check
// reports what a lock can't stop, as protect does without locking, sums
// them, compares with the sums the last one recorded, and names what
// changed. That's after the fact: git on the host may have run it
// already, and the VM may have put it back before it stopped.

// Watched is what the last watch found: a sum of each path a lock would
// cover, by its path relative to the project folder, of each file in its
// hooks folders, of .git/commondir, and of what the config hands over to
// the project folder (includes, core.hooksPath). Scanned is set once a
// check has recorded it, so the first check of a VM, or of one an older
// devvm made, records without reporting.
type Watched struct {
	Scanned bool              `json:"scanned,omitempty"`
	Sums    map[string]string `json:"sums,omitempty"`
}

// The watch's bounds: hooks folders and configs are small, so a folder
// with more entries, or a file larger, than these is the VM's doing, and
// is summed by what's cheap (its entries' count, its size, its inode and
// change time) past them, and named at each check. A warning names at most
// maxListed paths, and the rest by their count.
const (
	maxWatchEntries = 1000     // entries of one hooks folder
	maxWatchBytes   = 16 << 20 // bytes of one file
	maxWatchTotal   = 2000     // sums of the folder's own repo
	maxWatchNested  = 200      // sums of each repo nested in it
	maxListed       = 10       // paths one warning names
)

// WarnUnprotected warns, at each launch of the VM vm on a host without
// locks, that it can change the hooks and config of the folder's .git,
// which git on this machine runs, and names the repos nested in the
// folder that the last walk found (InVM.Nested): unless the folder has
// neither, or the user chose off (Chosen), which says they know. On a
// Linux host that locks now (Binds), a VM left off because the host
// couldn't lock when it was made, or by a devvm that didn't record the
// choice, is told it can be.
func (p *Protector) WarnUnprotected(vm *state.VM) {
	g, _ := Of(vm)
	repos := exists(p.git()) || len(g.Made.Repos) > 0
	if g.Mode == Clone || g.Mode == Off && g.Chosen || !repos && len(g.Nested) == 0 {
		return
	}
	_, binds := p.Locker.(Binds)
	switch {
	case !p.Locker.Locks():
		if repos {
			p.Warn(msg.GitUnprotected(vm.Name()))
		}
		p.warnNested(g.Nested, unprotected(vm.Name()))
		p.warnPast(vm.Name(), g.NestedMore, g.NestedPast)
	case binds && g.Mode == Off:
		p.Warn(msg.GitProtectAvailable(vm.Name()))
	}
}

// watcher is a protector's watch, on a host without locks.
type watcher struct{ *Protector }

// check is a check of a stopped VM's .git on a host without locks: what a
// lock can't stop, reported as protect reports it, without locking (a VM
// whose protection was never set up reports as a new VM does), then the
// watch, both from one scan.
func (w watcher) check(ctx context.Context, vm *state.VM, g InVM) error {
	why := whyCheck
	if !g.Made.Protected {
		why = whyNew
	}
	// Watching changes nothing in the folder, so its nested repos are
	// watched from the first check: each is recorded without naming.
	c := w.repos(g.Made, true, w.Nested)
	snaps := scanAll(ctx, c.trees)
	made := w.protect(ctx, vm.Name(), snaps, g, why, watching, c)
	made.Nested = true
	if err := record(ctx, vm, func(g *InVM) { g.Made = made }); err != nil {
		return err
	}
	if err := w.nested(ctx, vm, snaps, why); err != nil {
		return err
	}
	return w.watch(ctx, vm, snaps, g.Watched, unwatched(g.Made, made, c.gone))
}

// watch sums what a lock would cover in each repo snaps scanned, names
// what changed since the last check, once there was one, but in the
// nested repos skip (scanned for the first time, or gone), and what it
// can't fully watch, at every check, and records the sums.
func (w watcher) watch(ctx context.Context, vm *state.VM, snaps []snapshot, was Watched, skip []string) error {
	s := w.sums(snaps)
	now := Watched{Scanned: true, Sums: s.sums}
	if was.Scanned {
		roots := slices.Clone(skip)
		for _, s := range snaps {
			roots = append(roots, relTo(w.Dir, s.tree.Root))
		}
		changed := slices.DeleteFunc(changedPaths(was.Sums, now.Sums), func(path string) bool {
			return slices.Contains(skip, repoOf(path, roots))
		})
		if len(changed) > 0 {
			l, more := listed(changed)
			if more == 0 {
				w.Warn(msg.GitChanged(l, vm.Name()))
			} else {
				w.Warn(msg.GitChangedMore(more, l, vm.Name()))
			}
		}
	}
	if len(s.blind) > 0 {
		if l, more := listed(s.blind); more == 0 {
			w.Warn(msg.GitWatchBlind(l))
		} else {
			w.Warn(msg.GitWatchBlindMore(more, l))
		}
	}
	return record(ctx, vm, func(g *InVM) { g.Watched = now })
}

// repoOf is the deepest of roots, repos' top folders relative to the
// project folder, that path, relative to it too, is in; "" for none.
func repoOf(path string, roots []string) string {
	best := ""
	for _, r := range roots {
		if strings.HasPrefix(path, r+"/") && len(r) > len(best) {
			best = r
		}
	}
	return best
}

// listed is paths as a warning names them: at most maxListed, joined, and
// how many more there are.
func listed(paths []string) (string, int) {
	if len(paths) <= maxListed {
		return strings.Join(paths, ", "), 0
	}
	return strings.Join(paths[:maxListed], ", "), len(paths) - maxListed
}

// changedPaths returns the paths whose sums differ between was and now,
// those added or gone included, sorted.
func changedPaths(was, now map[string]string) []string {
	var out []string
	for path, sum := range now {
		if was[path] != sum {
			out = append(out, path)
		}
	}
	for path := range was {
		if _, ok := now[path]; !ok {
			out = append(out, path)
		}
	}
	slices.Sort(out)
	return out
}

// sums sums, in each repo snaps scanned, each path a lock would cover
// (paths), each file in its hooks folders, .git/commondir, there or not,
// and what the config hands over to the project folder
// (watchedDelegations), by path relative to the project folder, at most
// maxWatchTotal for the folder's own repo and maxWatchNested for each
// nested one. It never reads through a symlink that leads out of the
// project folder.
func (w watcher) sums(snaps []snapshot) summer {
	all := summer{sums: map[string]string{}}
	for _, snap := range snaps {
		s := w.repoSums(snap)
		maps.Copy(all.sums, s.sums)
		all.blind = append(all.blind, s.blind...)
	}
	slices.Sort(all.blind)
	all.blind = slices.Compact(all.blind)
	return all
}

// repoSums is sums of one repo.
func (w watcher) repoSums(snap snapshot) summer {
	bound := maxWatchNested
	if snap.tree.Root == w.Dir {
		bound = maxWatchTotal
	}
	s := summer{top: w.Dir, real: w.realInDir, max: bound, sums: map[string]string{}}
	todo := slices.Clone(snap.paths)
	if !isFile(snap.git) {
		todo = append(todo, filepath.Join(snap.git, "commondir"))
	}
	todo = append(todo, w.watchedDelegations(snap)...)
	done := map[string]bool{}
	for _, path := range todo {
		if !done[path] {
			done[path] = true
			s.add(path)
		}
	}
	slices.Sort(s.blind)
	s.blind = slices.Compact(s.blind)
	return s
}

// watchedDelegations are the files and folders in the project folder
// that the config hands over to git: include.path's and includeIf's
// files (delegations), and the folder core.hooksPath names, whose hooks
// git runs in place of .git/hooks.
func (w watcher) watchedDelegations(snap snapshot) []string {
	var out []string
	for _, d := range snap.ds {
		if d.kind == includeInside && d.path != "" {
			out = append(out, d.path)
		}
	}
	if hp := snap.hooksPath; hp != "" {
		hp = expandHome(w.Home, hp)
		if !filepath.IsAbs(hp) {
			hp = filepath.Join(snap.tree.Root, hp)
		}
		out = append(out, filepath.Clean(hp))
	}
	return out
}

// summer sums a watch's paths, bounded, and keeps those it can't watch
// fully (blind): a symlink or a path under one, a file past maxWatchBytes,
// a folder past maxWatchEntries, a path past max.
type summer struct {
	top   string // the project folder
	real  func(path string) bool
	max   int // how many sums it keeps: maxWatchTotal or maxWatchNested
	sums  map[string]string
	blind []string
}

// add sums what's at path, if anything: a missing path is absent from the
// sums, so one that appears is named.
func (s *summer) add(path string) {
	rel := relTo(s.top, path)
	fi, err := os.Lstat(path)
	switch {
	case err != nil:
	case fi.Mode()&fs.ModeSymlink != 0:
		s.put(rel, sumOf(path, fi)+s.within(path))
		s.blind = append(s.blind, rel)
	case !s.real(path):
		s.put(rel, "under a symlink"+s.within(path))
		s.blind = append(s.blind, rel)
	case fi.IsDir():
		s.hooks(path)
	default:
		s.file(rel, path, fi)
	}
}

// put records a sum, or past max, the path as one it can't watch.
func (s *summer) put(rel, sum string) {
	if len(s.sums) >= s.max {
		s.blind = append(s.blind, rel)
		return
	}
	s.sums[rel] = sum
}

// file sums a regular file, or anything else that isn't a folder or a
// symlink, by its type.
func (s *summer) file(rel, path string, fi fs.FileInfo) {
	if fi.Mode().IsRegular() && fi.Size() > maxWatchBytes {
		s.blind = append(s.blind, rel)
	}
	s.put(rel, sumOf(path, fi))
}

// within is what a symlink, or a path under one, holds, when it leads to
// a file or folder in the project folder, which the VM can change: its
// sum after ", ". A link out of the project folder isn't followed.
func (s *summer) within(path string) string {
	r, err := filepath.EvalSymlinks(path)
	if err != nil || !paths.Below(r, s.top) {
		return ""
	}
	fi, err := os.Lstat(r)
	switch {
	case err != nil:
		return ""
	case fi.IsDir():
		inner := summer{top: s.top, real: s.real, max: s.max, sums: map[string]string{}}
		inner.hooks(r)
		h := sha256.New()
		for _, k := range slices.Sorted(maps.Keys(inner.sums)) {
			h.Write([]byte(strconv.Itoa(len(k)) + ":" + k + "=" + inner.sums[k] + "\n"))
		}
		return ", folder " + hex.EncodeToString(h.Sum(nil))
	}
	return ", " + sumOf(r, fi)
}

// hooks sums each entry of a hooks folder below it, git's samples apart
// (git never runs them): first the entries at its top named as git's
// hooks (hostgit.HookEvents), which git runs, then the rest, at most
// maxWatchEntries in all. Past those, the folder is summed by its count,
// inode and change time, and named as one it can't watch.
func (s *summer) hooks(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	events := hostgit.HookEvents()
	slices.SortStableFunc(entries, func(a, b fs.DirEntry) int {
		return boolOrder(!slices.Contains(events, a.Name())) - boolOrder(!slices.Contains(events, b.Name()))
	})
	n := 0
	var sub []string
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		switch {
		case e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".sample"):
			continue
		case e.IsDir():
			sub = append(sub, path)
			continue
		}
		if n++; n > maxWatchEntries {
			s.over(dir)
			return
		}
		s.entry(path)
	}
	for _, d := range sub {
		err := filepath.WalkDir(d, func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".sample") {
				return nil //nolint:nilerr // unreadable: skipped, like find
			}
			if n++; n > maxWatchEntries {
				return fs.SkipAll
			}
			s.entry(path)
			return nil
		})
		if err != nil || n > maxWatchEntries {
			s.over(dir)
			return
		}
	}
}

// entry sums one entry of a hooks folder.
func (s *summer) entry(path string) {
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}
	rel := relTo(s.top, path)
	if fi.Mode()&fs.ModeSymlink != 0 {
		s.put(rel, sumOf(path, fi)+s.within(path))
		s.blind = append(s.blind, rel)
		return
	}
	s.file(rel, path, fi)
}

// over sums a hooks folder past maxWatchEntries by its count, inode and
// change time, and names it as one it can't watch.
func (s *summer) over(dir string) {
	rel := relTo(s.top, dir)
	sum := "more than " + strconv.Itoa(maxWatchEntries) + " entries"
	if fi, err := os.Lstat(dir); err == nil {
		sum += ", " + platform.ChangeID(fi)
	}
	s.put(rel, sum)
	s.blind = append(s.blind, rel)
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

// relTo is a path in the project folder top relative to it.
func relTo(top, path string) string { return strings.TrimPrefix(path, top+"/") }

// sumOf sums what's at path, which fi (from Lstat) describes: a regular
// file by its contents, read without following a symlink or waiting on a
// FIFO swapped in (safefile's), past
// maxWatchBytes by its size, inode and change time (which the VM can't set
// back), a symlink by where it points, and anything else by its type.
func sumOf(path string, fi fs.FileInfo) string {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		link, _ := os.Readlink(path)
		return "symlink " + link
	case !fi.Mode().IsRegular():
		return "type " + fi.Mode().Type().String()
	case fi.Size() > maxWatchBytes:
		return "size " + strconv.FormatInt(fi.Size(), 10) + ", " + platform.ChangeID(fi)
	}
	f, err := safefile.Open(path, safefile.Opts{})
	if err != nil {
		return "unreadable"
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, maxWatchBytes)); err != nil {
		return "unreadable"
	}
	return "sha256 " + hex.EncodeToString(h.Sum(nil))
}
