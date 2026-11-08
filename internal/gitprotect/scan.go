package gitprotect

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/devvmhq/devvm/internal/hostsafety"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/safefile"
)

// tree is a repo in a project folder whose .git the protection finds
// (scan): the folder's own, or one nested in it. It has two bounds. Its
// own top folder, Root, is where its .git, .git/modules and a relative
// core.hooksPath are; the project folder, Dir, which the VM mounts, is
// what the VM can write, so what's in it counts as inside (realInDir,
// into, hooksPathInside) and a submodule's git dir outside it can't be
// locked (embeddedIn).
type tree struct {
	Dir  string // the project folder, a physical path
	Root string // the repo's top folder: Dir, or a folder below it
	Home string // for ~ in git's paths
	// budget is the submodule git dirs the check's scans may still follow,
	// shared by every repo they scan (scanAll); nil for this scan's own
	// maxEmbedded alone.
	budget *budget
}

// budget is how many submodule git dirs one check's scans follow in all,
// so the VM can't make a check slow with many repos full of them.
type budget struct{ left atomic.Int64 }

// newBudget is the budget of one check: maxChecked git dirs.
func newBudget() *budget {
	b := &budget{}
	b.left.Store(maxChecked)
	return b
}

// take takes one from the budget, and reports whether there was one.
func (b *budget) take() bool { return b == nil || b.left.Add(-1) >= 0 }

// maxChecked is how many submodule git dirs one check follows across
// every repo it scans.
const maxChecked = 1000

// snapshot is what one scan of a repo's .git found: what locking,
// unlocking, the watch and the reports all go by, found once per check.
type snapshot struct {
	tree   tree   // the repo it scanned
	git    string // .git
	exists bool   // whether there's anything at .git
	id     string // .git's identity (folderID)
	// commondir is set when .git is a folder with a commondir file, which
	// points git elsewhere for config and hooks; target is what it says.
	commondir bool
	target    string
	strays    []string // linked worktrees whose commondir leads elsewhere (worktreeStrays)
	emb       embeds
	dirs      []string // the git dirs locked besides .git (lockedDirs)
	paths     []string // what to lock (tree.paths)
	ds        []delegation
	// hooksPath is core.hooksPath as written, when it leads into the
	// project folder (hooksPathInside); else "".
	hooksPath string
	// covered is the git dirs, resolved, whose hooks and config this scan
	// protects whole (coveredDirs), which a nested repo's .git can be left
	// to.
	covered []string
}

// scan finds what the protection goes by.
func (t tree) scan(ctx context.Context) snapshot {
	g := t.git()
	s := snapshot{tree: t, git: g, exists: exists(g), id: folderID(g)}
	if isDir(g) && exists(filepath.Join(g, "commondir")) {
		target, _ := gitFile(filepath.Join(g, "commondir"))
		s.commondir, s.target = true, strings.TrimSpace(string(target))
	}
	s.strays = t.worktreeStrays()
	s.emb = t.embedded(ctx)
	s.dirs = t.lockedDirs(s.emb)
	s.paths = t.paths(ctx, s.emb)
	s.ds = t.delegations(ctx, s.emb)
	if hp := gitConfig(ctx, filepath.Join(g, "config"), "core.hooksPath"); hp != "" && t.hooksPathInside(hp) {
		s.hooksPath = hp
	}
	s.covered = t.coveredDirs(s.strays)
	return s
}

// coveredDirs are the git dirs, resolved, whose hooks and config the
// repo's locks cover whole: .git, the git dirs under .git/modules that
// read their config from themselves (no commondir that leads elsewhere),
// and the linked worktrees under them whose commondir leads back (not in
// strays), each a real folder whose commondir a lock can reach. A git dir
// the VM made elsewhere under .git, such as .git/evil, or a worktree that
// is a symlink to one, isn't one: nothing locks it.
func (t tree) coveredDirs(strays []string) []string {
	g := t.git()
	if !isDir(g) {
		return nil
	}
	var out []string
	add := func(d string) {
		if r, err := filepath.EvalSymlinks(d); err == nil {
			out = append(out, r)
		}
	}
	add(g)
	for _, d := range t.gitDirs() {
		if c, ok := commonOf(d); d != g && ok && c == resolved(d) {
			add(d)
		}
		for _, w := range worktrees(d) {
			// A worktree that's a symlink, or whose commondir is under one,
			// can't be locked (realInDir), so the VM can point it anywhere
			// once the check is done: what names it isn't covered.
			if !slices.Contains(strays, w) && !isLink(w) && t.realInDir(filepath.Join(w, "commondir")) {
				add(w)
			}
		}
	}
	return out
}

// resolved is path with its symlinks resolved, or "" when it can't be.
func resolved(path string) string {
	r, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return r
}

func (t tree) git() string { return filepath.Join(t.Root, ".git") }

// lockedDirs are the git dirs protect locks besides .git: those under
// .git/modules (gitDirs) and the submodules' in the working tree.
func (t tree) lockedDirs(emb embeds) []string {
	var out []string
	for _, d := range t.gitDirs() {
		if d != t.git() {
			out = append(out, d)
		}
	}
	return append(out, emb.inside...)
}

// abs is git dirs as Made records them, relative to the project folder,
// as absolute paths.
func (t tree) abs(rel []string) []string {
	out := make([]string, 0, len(rel))
	for _, r := range rel {
		out = append(out, filepath.Join(t.Dir, r))
	}
	return out
}

// rel is git dirs in the project folder as Made records them: relative
// to it.
func (t tree) rel(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, relTo(t.Dir, d))
	}
	return out
}

// gitDirs returns the repo's git dirs: .git and its submodules', anything
// under .git/modules that git would take for one (hostsafety.IsGitDir).
func (t tree) gitDirs() []string {
	g := t.git()
	if !isDir(g) {
		return nil
	}
	dirs := []string{g}
	_ = filepath.WalkDir(filepath.Join(g, "modules"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() != "HEAD" {
			return nil //nolint:nilerr // unreadable: skipped, like find
		}
		if dir := filepath.Dir(path); hostsafety.IsGitDir(dir) {
			dirs = append(dirs, dir)
		}
		return nil
	})
	return dirs
}

// embeds are the submodule git dirs of the repo's working tree: inside
// the project folder, or (with the submodule's folder) outside it; and
// the folders inside it that a submodule names as its git dir but that
// aren't one (hostsafety.IsGitDir), which nothing locks.
type embeds struct {
	inside  []string
	outside [][2]string     // the submodule's folder, its git dir
	notGit  [][2]string     // the submodule's folder, the folder it names
	seen    map[string]bool // the git dirs followed, resolved
	// bounded is set when the scan stopped following them, past
	// maxEmbedded or the check's budget (git_scan_bounded).
	bounded bool
}

// embedded finds the submodules whose git dir is in the working tree: a
// gitlink in the index (which the VM writes, to commit) whose folder has
// a .git folder of its own, or a .git file pointing elsewhere. Host git
// status reads their config, and the VM can make one at any time.
// Submodules' own indexes are read too (host git status recurses into
// nested submodules). The index is read with fsmonitor off; ls-files
// never enters a submodule.
func (t tree) embedded(ctx context.Context) embeds {
	var e embeds
	g := t.git()
	if isDir(g) && !isLink(g) {
		e.seen = map[string]bool{}
		if r, err := filepath.EvalSymlinks(g); err == nil {
			e.seen[r] = true
		}
		t.embeddedIn(ctx, &e, g, t.Root, 0)
	}
	return e
}

// maxEmbedded is how many submodule git dirs one repo's scan follows.
const maxEmbedded = 1000

func (t tree) embeddedIn(ctx context.Context, e *embeds, gitDir, worktree string, depth int) {
	if depth >= maxDepth {
		return
	}
	out := git(ctx, "-c", "core.fsmonitor=false", "--git-dir="+gitDir, "--work-tree="+worktree, "ls-files", "-s", "-z")
	for entry := range bytes.SplitSeq(out, []byte{0}) {
		info, path, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok || !bytes.HasPrefix(info, []byte("160000 ")) {
			continue
		}
		w := filepath.Join(worktree, string(path))
		g := filepath.Join(w, ".git")
		var d string
		switch {
		case isLink(g):
			continue
		case isDir(g):
			d = g
		case isFile(g):
			d = gitdirOf(g, w)
			if d == "" {
				continue
			}
		default:
			continue
		}
		if !paths.Below(d, t.Dir) {
			// Nothing to lock here: delegations names it.
			e.outside = append(e.outside, [2]string{w, d})
			continue
		}
		if !hostsafety.IsGitDir(d) {
			// The VM can point a .git file, or make a .git folder, at
			// any folder in the project: locking its hooks would lock
			// the project's own files.
			e.notGit = append(e.notGit, [2]string{w, d})
			continue
		}
		// Each git dir once: the VM can make gitlinks that lead back to
		// their own git dir, or through a symlinked folder to one seen.
		r, err := filepath.EvalSymlinks(d)
		if err != nil || e.seen[r] {
			continue
		}
		if len(e.seen) >= maxEmbedded || !t.budget.take() {
			e.bounded = true
			return
		}
		e.seen[r] = true
		if !paths.Below(d, filepath.Join(t.git(), "modules")) { // gitDirs has those
			e.inside = append(e.inside, d)
		}
		t.embeddedIn(ctx, e, d, w, depth+1)
	}
}

// paths returns what to lock: the hooks and config of the repo and of its
// submodules (lock creates a missing hooks folder empty, or the VM could
// make one), config.worktree with extensions.worktreeConfig (lock creates
// it), and what linked worktrees elsewhere read from here (commondir,
// config.worktree); or, in a linked worktree, its .git file.
func (t tree) paths(ctx context.Context, emb embeds) []string {
	g := t.git()
	if isFile(g) {
		return []string{g}
	}
	var out []string
	for _, d := range t.gitDirs() {
		out = append(out, filepath.Join(d, "hooks"))
		if isFile(filepath.Join(d, "config")) {
			out = append(out, filepath.Join(d, "config"))
		}
		wtc := worktreeConfig(ctx, d)
		if wtc {
			out = append(out, filepath.Join(d, "config.worktree"))
		}
		for _, w := range worktrees(d) {
			if isFile(filepath.Join(w, "commondir")) {
				out = append(out, filepath.Join(w, "commondir"))
			}
			if wtc {
				out = append(out, filepath.Join(w, "config.worktree"))
			}
		}
	}
	for _, d := range emb.inside {
		out = append(out, filepath.Join(d, "hooks"))
		if isFile(filepath.Join(d, "config")) {
			out = append(out, filepath.Join(d, "config"))
		}
	}
	return out
}

// worktreeStrays returns the linked worktrees (.git/worktrees/<n>) whose
// commondir doesn't lead back to their git dir: git in that worktree reads
// config and hooks from wherever it points (without one, from <n> itself).
func (t tree) worktreeStrays() []string {
	var out []string
	for _, d := range t.gitDirs() {
		physical, _ := filepath.EvalSymlinks(d)
		for _, w := range worktrees(d) {
			if r, ok := commonOf(w); !ok || r != physical {
				out = append(out, w)
			}
		}
	}
	return out
}

// hooksPathInside reports whether core.hooksPath leads into the project
// folder, where the VM can edit the hooks. Relative paths are from the
// working tree. A path that isn't an existing folder counts by name and
// resolved as far as it exists.
func (t tree) hooksPathInside(hp string) bool {
	hp = expandHome(t.Home, hp)
	if !filepath.IsAbs(hp) {
		return true
	}
	if r, err := filepath.EvalSymlinks(hp); err == nil && isDir(r) {
		return paths.Within(r, t.Dir)
	}
	return paths.Within(filepath.Clean(hp), t.Dir) || paths.Within(physicalParent(hp), t.Dir)
}

// delegations finds what the locked config and hooks hand over to files
// the VM can edit: include.path and includeIf.*.path that lead into the
// project folder (relative to the config's folder, as git reads them), and
// hooks that are symlinks into it; and the submodules whose git dir is
// outside the project, which devvm can't lock (another project's VM may
// write it), or a folder inside it that isn't a git dir, which devvm
// doesn't lock.
func (t tree) delegations(ctx context.Context, emb embeds) []delegation {
	var out []delegation
	for _, d := range append(t.gitDirs(), emb.inside...) {
		cfg := filepath.Join(d, "config")
		list := git(ctx, "config", "--file", cfg, "-z", "--get-regexp", `^include(if\..*)?\.path$`)
		for rec := range bytes.SplitSeq(list, []byte{0}) {
			_, v, ok := bytes.Cut(rec, []byte{'\n'})
			if !ok || len(v) == 0 {
				continue
			}
			path := expandHome(t.Home, string(v))
			if !filepath.IsAbs(path) {
				path = filepath.Join(d, path)
			}
			if t.into(path) {
				out = append(out, delegation{includeInside, cfg, string(v), filepath.Clean(path)})
			}
		}
		_ = filepath.WalkDir(filepath.Join(d, "hooks"), func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.Type()&fs.ModeSymlink == 0 {
				return nil //nolint:nilerr // unreadable: skipped, like find
			}
			link, err := os.Readlink(path)
			if err != nil {
				return nil //nolint:nilerr // gone meanwhile
			}
			target := link
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			if t.into(target) {
				out = append(out, delegation{hookLinkInside, path, link, ""})
			}
			return nil
		})
	}
	for _, o := range emb.outside {
		out = append(out, delegation{gitdirOutside, o[0], o[1], ""})
	}
	for _, n := range emb.notGit {
		out = append(out, delegation{gitdirNotGit, n[0], n[1], ""})
	}
	return out
}

// into reports whether path, resolved, is in the project folder. A path
// whose folder is missing counts by name too: the VM can make the folder
// or change the symlinks it would be under.
func (t tree) into(path string) bool {
	if _, err := os.Stat(filepath.Dir(path)); err == nil {
		return paths.Below(paths.PhysicalParent(path), t.Dir)
	}
	return paths.Below(filepath.Clean(path), t.Dir) || paths.Below(physicalParent(path), t.Dir)
}

// realInDir reports whether path is a real path in the project folder,
// with no symlink in it below the folder. The VM can plant symlinks under
// .git: creating a file or setting flags through one would act outside the
// project, and chflags on a symlink itself does nothing.
func (t tree) realInDir(path string) bool {
	if !paths.Below(path, t.Dir) || isLink(path) {
		return false
	}
	parent := filepath.Dir(path)
	r, err := filepath.EvalSymlinks(parent)
	return err == nil && r == parent
}

// worktrees returns a git dir's linked worktrees' folders, as the glob
// worktrees/*/ has them: folders and links to folders, sorted, without
// hidden ones.
func worktrees(gitDir string) []string {
	entries, _ := os.ReadDir(filepath.Join(gitDir, "worktrees"))
	var out []string
	for _, e := range entries {
		w := filepath.Join(gitDir, "worktrees", e.Name())
		if !strings.HasPrefix(e.Name(), ".") && isDir(w) {
			out = append(out, w)
		}
	}
	return out
}

// gitFile reads one of git's own small files, which the VM can change: a
// .git file, a commondir, a worktree's gitdir. It follows a symbolic link,
// as git does, but reads only a regular file of at most maxWatchBytes
// (safefile's), so a FIFO there can't hold devvm.
func gitFile(path string) ([]byte, error) {
	return safefile.Read(path, maxWatchBytes, safefile.Opts{Follow: true})
}

// gitdirOf returns the git dir a .git file points to, resolved, or "".
func gitdirOf(file, worktree string) string {
	data, err := gitFile(file)
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		t, ok := strings.CutPrefix(line, "gitdir: ")
		if !ok {
			continue
		}
		return resolvedDir(worktree, t)
	}
	return ""
}

// resolvedDir returns p, relative to dir unless it's absolute, with its
// symlinks resolved, when it's a folder, or "".
func resolvedDir(dir, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	r, err := filepath.EvalSymlinks(p)
	if err != nil || !isDir(r) {
		return ""
	}
	return r
}

// commonOf returns where git in the git dir g reads the repo's config,
// objects and refs from: its commondir, resolved (relative to g, as git
// reads it), else g itself, resolved. It's false when that isn't there.
func commonOf(g string) (string, bool) {
	target := g
	if c, err := gitFile(filepath.Join(g, "commondir")); err == nil {
		target = strings.TrimRight(string(c), "\n")
		if !filepath.IsAbs(target) {
			target = filepath.Join(g, target)
		}
	}
	r, err := filepath.EvalSymlinks(target)
	return r, err == nil
}

// expandHome expands ~ as git does, to home.
func expandHome(home, path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

// physicalParent is path with its folder resolved as far as it exists
// (paths.Physical), so a missing folder under a symlinked prefix still
// resolves, and its last element as named.
func physicalParent(path string) string {
	return filepath.Join(paths.Physical(filepath.Dir(path)), filepath.Base(path))
}
