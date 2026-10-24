// Package gitprotect keeps a VM from changing the parts of its project's
// .git that run code on the host. The project folder is mounted read-write,
// so without it the VM could add a hook, or set core.fsmonitor, hooksPath or
// sshCommand in .git/config, and the host's next git command would run it.
// A read-only mount wouldn't hold (root in the VM can unmount it), so on
// macOS devvm locks those paths with the user-immutable flag (chflags uchg),
// which the VM can't clear. Linux hosts have no unprivileged flag like it.
//
// A lock can't stop the VM from swapping the whole .git, or from pointing
// git elsewhere (commondir), so devvm reports what it finds and never
// repairs .git itself: the user decides.
package gitprotect

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// Keys of a VM's state that git protection keeps.
const (
	KeyGitProtect     = "git_protect"     // on or off
	KeyGitInode       = "git_inode"       // .git's inode when it was last locked
	KeyGitEmbedded    = "git_embedded"    // submodule git dirs in the working tree (JSON, relative paths)
	KeyGitModules     = "git_modules"     // git dirs under .git/modules (JSON, relative paths)
	KeyGitDelegations = "git_delegations" // a hash of what the locked files hand over to the project folder
)

// maxDepth is how deep submodules of submodules are followed.
const maxDepth = 8

// Protector protects one project folder's .git.
type Protector struct {
	Dir  string // the project folder, a physical path
	Home string // for ~ in git's paths
	// OS is the host's, as Go names it: locks need darwin.
	OS string
	// Chflags sets file flags on a path and everything under it, following
	// no symlink (platform.Chflags).
	Chflags func(ctx context.Context, flags, path string) error
	// Locked reports whether a path has the user-immutable flag, not
	// following a symlink (platform.Immutable).
	Locked func(path string) bool
	// Warn prints a warning (ui.Warn).
	Warn func(msg.M)
}

// New returns the protector for the project folder dir.
func New(dir string) *Protector {
	return &Protector{
		Dir: dir, Home: os.Getenv("HOME"), OS: runtime.GOOS,
		Chflags: platform.Chflags, Locked: platform.Immutable, Warn: ui.Warn,
	}
}

// NewVM sets up protection for a VM just made for the folder, stopped:
// off when asked, or on hosts without locks (saying so when there's a
// .git); otherwise on, with .git locked and what a lock can't cover
// reported.
func (p *Protector) NewVM(ctx context.Context, vm *state.VM, off bool) error {
	switch {
	case off:
		return vm.Set(KeyGitProtect, "off")
	case p.OS != "darwin":
		if err := vm.Set(KeyGitProtect, "off"); err != nil {
			return err
		}
		if exists(p.git()) {
			p.Warn(msg.GitProtectNeedsMacosNew())
		}
		return nil
	}
	if err := vm.Set(KeyGitProtect, "on"); err != nil {
		return err
	}
	g := p.git()
	if !exists(g) {
		return nil
	}
	if isDir(g) && exists(filepath.Join(g, "commondir")) {
		target, _ := os.ReadFile(filepath.Join(g, "commondir")) //nolint:gosec // the project's own
		p.Warn(msg.GitCommondir(msg.Shown(g), msg.Shown(strings.TrimSpace(string(target)))))
	}
	for _, w := range p.worktreeStrays() {
		p.Warn(msg.GitWorktreeStray(msg.Shown(w)))
	}
	emb := p.embedded(ctx)
	if err := vm.Set(KeyGitEmbedded, p.relList(emb.inside)); err != nil {
		return err
	}
	var mods []string
	for _, d := range p.gitDirs() {
		if d != g {
			mods = append(mods, d)
		}
	}
	if err := vm.Set(KeyGitModules, p.relList(mods)); err != nil {
		return err
	}
	if err := p.lock(ctx, vm, emb); err != nil {
		return err
	}
	if hp := gitConfig(ctx, filepath.Join(g, "config"), "core.hooksPath"); hp != "" && p.hooksPathInside(hp) {
		p.Warn(msg.GitHooksPath(msg.Shown(hp)))
	}
	ds := p.delegations(ctx, emb)
	p.warnDelegations(ds)
	return vm.Set(KeyGitDelegations, delegationsSum(ds))
}

func (p *Protector) warnDelegations(ds []delegation) {
	for _, d := range ds {
		switch d.kind {
		case "include":
			p.Warn(msg.GitIncludeInside(msg.Shown(d.where), msg.Shown(d.target)))
		case "hook":
			p.Warn(msg.GitHookLinkInside(msg.Shown(d.where), msg.Shown(d.target)))
		case "outside":
			p.Warn(msg.GitGitdirOutside(msg.Shown(d.target), msg.Shown(d.where)))
		}
	}
}

// Check runs on every launch of an existing VM: it locks again what's new
// (a submodule, a repo made since), and reports what a lock can't stop,
// once each: a .git replaced since it was locked (then nothing else), a
// repo that appeared, submodule git dirs that appeared, hooks in a hooks
// folder that wasn't locked, and includes or hook links into the project
// folder when they change. Locks are set only while the VM is stopped
// (running), so nothing in it can race them; a running VM gets them at
// its next start. A VM whose protection was never set up (its creation
// stopped short) gets it as a new VM does.
func (p *Protector) Check(ctx context.Context, vm *state.VM, running bool) error {
	switch vm.Get(KeyGitProtect) {
	case "":
		if running {
			return nil
		}
		return p.NewVM(ctx, vm, false)
	case "on":
	default:
		return nil
	}
	if p.OS != "darwin" {
		return nil
	}
	g := p.git()
	if !exists(g) {
		return nil
	}
	was := vm.Get(KeyGitInode)
	if was != "" && was != inode(g) {
		p.Warn(msg.GitReplaced(msg.Shown(g)))
		return nil
	}
	if isDir(g) && exists(filepath.Join(g, "commondir")) {
		target, _ := os.ReadFile(filepath.Join(g, "commondir")) //nolint:gosec // the project's own
		p.Warn(msg.GitCommondir(msg.Shown(g), msg.Shown(strings.TrimSpace(string(target)))))
	}
	for _, w := range p.worktreeStrays() {
		p.Warn(msg.GitWorktreeStray(msg.Shown(w)))
	}
	if was == "" {
		p.Warn(msg.GitAppeared())
	}
	emb := p.embedded(ctx)
	seen, _ := p.list(vm.Get(KeyGitEmbedded))
	for _, d := range emb.inside {
		if !slices.Contains(seen, d) {
			p.Warn(msg.GitEmbeddedNew(msg.Shown(d)))
		}
	}
	if err := vm.Set(KeyGitEmbedded, p.relList(emb.inside)); err != nil {
		return err
	}
	// Nothing recorded yet: nothing to compare with.
	seen, recorded := p.list(vm.Get(KeyGitModules))
	var mods []string
	for _, d := range p.gitDirs() {
		if d == g {
			continue
		}
		mods = append(mods, d)
		if recorded && !slices.Contains(seen, d) {
			p.Warn(msg.GitEmbeddedNew(msg.Shown(d)))
		}
	}
	if err := vm.Set(KeyGitModules, p.relList(mods)); err != nil {
		return err
	}
	// Hooks in a hooks folder that isn't locked appeared since the last
	// lock (there was no folder to lock, or it's a new submodule's), so the
	// VM may have made them.
	for _, path := range p.paths(ctx, emb) {
		if filepath.Base(path) == "hooks" && isDir(path) && !isLink(path) && !p.Locked(path) && hasHooks(path) {
			p.Warn(msg.GitHooksUnlocked(msg.Shown(path)))
		}
	}
	if !running {
		if err := p.lock(ctx, vm, emb); err != nil {
			return err
		}
	}
	ds := p.delegations(ctx, emb)
	sum := delegationsSum(ds)
	if sum != vm.Get(KeyGitDelegations) {
		p.warnDelegations(ds)
	}
	return vm.Set(KeyGitDelegations, sum)
}

// list reads a state value relList wrote, as absolute paths; ok is false
// when there's none.
func (p *Protector) list(v string) (paths []string, ok bool) {
	var rel []string
	if v == "" || json.Unmarshal([]byte(v), &rel) != nil {
		return nil, false
	}
	for _, r := range rel {
		paths = append(paths, filepath.Join(p.Dir, r))
	}
	return paths, true
}

// hasHooks reports whether a hooks folder has a file that isn't one of
// git's samples.
func hasHooks(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err == nil && e.Type().IsRegular() && !strings.HasSuffix(e.Name(), ".sample") {
			found = true
			return fs.SkipAll
		}
		return nil //nolint:nilerr // unreadable: skipped, like find
	})
	return found
}

// Unlock takes the locks off, for a VM that's being deleted. Like
// locking, it never acts through a symlink.
func (p *Protector) Unlock(ctx context.Context) {
	if p.OS != "darwin" {
		return
	}
	for _, path := range p.paths(ctx, p.embedded(ctx)) {
		if _, err := os.Lstat(path); err == nil && p.realInDir(path) {
			p.flag(ctx, "nouchg", path)
		}
	}
}

func (p *Protector) git() string { return filepath.Join(p.Dir, ".git") }

// relList is paths in the project folder as a state value: a JSON list of
// their paths relative to it.
func (p *Protector) relList(paths []string) string {
	rel := []string{}
	for _, d := range paths {
		rel = append(rel, strings.TrimPrefix(d, p.Dir+"/"))
	}
	b, err := json.Marshal(rel)
	if err != nil {
		panic(err) // strings: always encodable
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func isDir(path string) bool { fi, err := os.Stat(path); return err == nil && fi.IsDir() }

func isLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&fs.ModeSymlink != 0
}

func isFile(path string) bool { fi, err := os.Stat(path); return err == nil && fi.Mode().IsRegular() }

// git runs git and returns its output; nothing when it fails.
func git(ctx context.Context, args ...string) []byte {
	out, err := exec.CommandContext(ctx, "git", args...).Output() //nolint:gosec // devvm's own arguments
	if err != nil {
		return nil
	}
	return out
}

// gitConfig returns a key's value in one config file, or "".
func gitConfig(ctx context.Context, file, key string, opts ...string) string {
	args := append([]string{"config", "--file", file}, opts...)
	out := git(ctx, append(args, "--get", key)...)
	v, _, _ := strings.Cut(string(out), "\n")
	return v
}

func worktreeConfig(ctx context.Context, gitDir string) bool {
	return gitConfig(ctx, filepath.Join(gitDir, "config"), "extensions.worktreeConfig", "--bool") == "true"
}

// gitDirs returns the repo's git dirs: .git and its submodules', anything
// under .git/modules that git would take for one: a HEAD (a file, or a
// symlink, which git still accepts) next to objects and refs.
func (p *Protector) gitDirs() []string {
	g := p.git()
	if !isDir(g) {
		return nil
	}
	dirs := []string{g}
	_ = filepath.WalkDir(filepath.Join(g, "modules"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() != "HEAD" || !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil //nolint:nilerr // unreadable: skipped, like find
		}
		if dir := filepath.Dir(path); isDir(filepath.Join(dir, "objects")) && isDir(filepath.Join(dir, "refs")) {
			dirs = append(dirs, dir)
		}
		return nil
	})
	return dirs
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

// embeds are the submodule git dirs of the repo's working tree: inside
// the project folder, or (with the submodule's folder) outside it.
type embeds struct {
	inside  []string
	outside [][2]string // the submodule's folder, its git dir
}

// embedded finds the submodules whose git dir is in the working tree: a
// gitlink in the index (which the VM writes, to commit) whose folder has
// a .git folder of its own, or a .git file pointing elsewhere. Host git
// status reads their config, and the VM can make one at any time.
// Submodules' own indexes are read too (host git status recurses into
// nested submodules). The index is read with fsmonitor off; ls-files
// never enters a submodule.
func (p *Protector) embedded(ctx context.Context) embeds {
	var e embeds
	g := p.git()
	if isDir(g) && !isLink(g) {
		p.embeddedIn(ctx, &e, g, p.Dir, 0)
	}
	return e
}

func (p *Protector) embeddedIn(ctx context.Context, e *embeds, gitDir, worktree string, depth int) {
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
		var t string
		switch {
		case isLink(g):
			continue
		case isDir(g):
			t = g
		case isFile(g):
			t = gitdirOf(g, w)
			if t == "" {
				continue
			}
		default:
			continue
		}
		if !strings.HasPrefix(t, p.Dir+"/") {
			// Nothing to lock here: delegations names it.
			e.outside = append(e.outside, [2]string{w, t})
			continue
		}
		if !strings.HasPrefix(t, filepath.Join(p.Dir, ".git", "modules")+"/") { // gitDirs has those
			e.inside = append(e.inside, t)
		}
		p.embeddedIn(ctx, e, t, w, depth+1)
	}
}

// gitdirOf returns the git dir a .git file points to, resolved, or "".
func gitdirOf(file, worktree string) string {
	data, err := os.ReadFile(file) //nolint:gosec // the project's own
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		t, ok := strings.CutPrefix(line, "gitdir: ")
		if !ok {
			continue
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(worktree, t)
		}
		r, err := filepath.EvalSymlinks(t)
		if err != nil || !isDir(r) {
			return ""
		}
		return r
	}
	return ""
}

// paths returns what to lock: the hooks and config of the repo and of its
// submodules (lock creates a missing hooks folder empty, or the VM could
// make one), config.worktree with extensions.worktreeConfig (lock creates
// it), and what linked worktrees elsewhere read from here (commondir,
// config.worktree); or, in a linked worktree, its .git file.
func (p *Protector) paths(ctx context.Context, emb embeds) []string {
	g := p.git()
	if isFile(g) {
		return []string{g}
	}
	var out []string
	for _, d := range p.gitDirs() {
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
func (p *Protector) worktreeStrays() []string {
	var out []string
	for _, d := range p.gitDirs() {
		physical, _ := filepath.EvalSymlinks(d)
		for _, w := range worktrees(d) {
			c, _ := os.ReadFile(filepath.Join(w, "commondir")) //nolint:gosec // the project's own
			target := strings.TrimRight(string(c), "\n")
			if !filepath.IsAbs(target) {
				target = filepath.Join(w, target)
			}
			if r, err := filepath.EvalSymlinks(target); err != nil || r != physical {
				out = append(out, w)
			}
		}
	}
	return out
}

// hooksPathInside reports whether core.hooksPath leads into the project
// folder, where the VM can edit the hooks. Relative paths are from the
// working tree.
func (p *Protector) hooksPathInside(hp string) bool {
	hp = p.expandHome(hp)
	if !filepath.IsAbs(hp) {
		return true
	}
	if r, err := filepath.EvalSymlinks(hp); err == nil && isDir(r) {
		hp = r
	}
	hp = filepath.Clean(hp)
	return hp == p.Dir || strings.HasPrefix(hp, p.Dir+"/")
}

// expandHome expands ~ as git does.
func (p *Protector) expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		return p.Home + path[1:]
	}
	return path
}

// delegation is something the locked files hand over to a file the VM can
// edit, or a git dir devvm can't lock.
type delegation struct {
	kind   string // include, hook or outside
	where  string // the config, the hook, or the submodule's folder
	target string // the include's path or the hook's link as written, or the git dir
}

// delegations finds what the locked config and hooks hand over to files
// the VM can edit: include.path and includeIf.*.path that lead into the
// project folder (relative to the config's folder, as git reads them), and
// hooks that are symlinks into it; and the submodules whose git dir is
// outside the project, which devvm can't lock (another project's VM may
// write it).
func (p *Protector) delegations(ctx context.Context, emb embeds) []delegation {
	var out []delegation
	for _, d := range append(p.gitDirs(), emb.inside...) {
		cfg := filepath.Join(d, "config")
		list := git(ctx, "config", "--file", cfg, "-z", "--get-regexp", `^include(if\..*)?\.path$`)
		for rec := range bytes.SplitSeq(list, []byte{0}) {
			_, v, ok := bytes.Cut(rec, []byte{'\n'})
			if !ok || len(v) == 0 {
				continue
			}
			path := p.expandHome(string(v))
			if !filepath.IsAbs(path) {
				path = filepath.Join(d, path)
			}
			if p.into(path) {
				out = append(out, delegation{"include", cfg, string(v)})
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
			if p.into(target) {
				out = append(out, delegation{"hook", path, link})
			}
			return nil
		})
	}
	for _, o := range emb.outside {
		out = append(out, delegation{"outside", o[0], o[1]})
	}
	return out
}

// into reports whether path, resolved, is in the project folder.
func (p *Protector) into(path string) bool {
	return strings.HasPrefix(resolvedPath(path), p.Dir+"/")
}

// resolvedPath returns path with its folder's symlinks resolved, or, when
// the folder doesn't exist (yet: the VM could make it), path with . and ..
// resolved by name.
func resolvedPath(path string) string {
	if d, err := filepath.EvalSymlinks(filepath.Dir(path)); err == nil {
		return filepath.Join(d, filepath.Base(path))
	}
	return filepath.Clean(path)
}

func delegationsSum(ds []delegation) string {
	h := sha256.New()
	for _, d := range ds {
		for _, f := range []string{d.kind, d.where, d.target} {
			h.Write([]byte(strconv.Itoa(len(f)) + ":" + f))
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// realInDir reports whether path is a real path in the project folder,
// with no symlink in it below the folder. The VM can plant symlinks under
// .git: creating a file or setting flags through one would act outside the
// project, and chflags on a symlink itself does nothing.
func (p *Protector) realInDir(path string) bool {
	if !strings.HasPrefix(path, p.Dir+"/") || isLink(path) {
		return false
	}
	parent := filepath.Dir(path)
	r, err := filepath.EvalSymlinks(parent)
	return err == nil && r == parent
}

// lock locks the paths to protect, which only happens while the VM is
// stopped, so nothing in it can race the checks; then it records .git's
// inode.
func (p *Protector) lock(ctx context.Context, vm *state.VM, emb embeds) error {
	for _, path := range p.paths(ctx, emb) {
		if !p.realInDir(path) {
			p.Warn(msg.GitNotLocked(msg.Shown(path)))
			continue
		}
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			// So the VM can't create it. O_EXCL and mkdir fail on any
			// existing name, and neither follows a symlink.
			switch filepath.Base(path) {
			case "config.worktree":
				if f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil { //nolint:gosec // checked by realInDir
					_ = f.Close()
				}
			case "hooks":
				_ = os.Mkdir(path, 0o700)
			}
		}
		if _, err := os.Lstat(path); err == nil {
			p.flag(ctx, "uchg", path)
		}
	}
	return vm.Set(KeyGitInode, inode(p.git()))
}

// flag sets a flag on a path realInDir passed, then checks it again.
// Between the check and chflags, a running VM could swap a folder above the
// path for a symlink to somewhere else: a path that moved meanwhile is
// unlocked again where it landed, and reported.
func (p *Protector) flag(ctx context.Context, flags, path string) bool {
	before := devIno(path)
	if before == "" {
		return true
	}
	_ = p.Chflags(ctx, flags, path)
	if devIno(path) == before && p.realInDir(path) {
		return true
	}
	if flags == "uchg" {
		_ = p.Chflags(ctx, "nouchg", path)
	}
	p.Warn(msg.GitLockRaced(msg.Shown(path)))
	return false
}

// devIno is what's at path (not following a symlink) as dev:ino, or "".
func devIno(path string) string {
	fi, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	return platform.DevIno(fi)
}

func inode(path string) string {
	_, ino, _ := strings.Cut(devIno(path), ":")
	return ino
}
