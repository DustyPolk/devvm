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
//
// Each check scans .git once (scan: a snapshot), which locking, unlocking,
// the watch and the reports (a table, reports) go by; the host's Locker
// does the locking.
package gitprotect

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/devvmhq/devvm/internal/hostgit"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/state"
)

// Section is the name of the .git protection section in a VM's state.
const Section = "git"

// InVM is a VM's .git protection section: its mode, as asked (recorded
// before its backend makes it, so it's never unset), whether the user
// chose it, the mode chosen while it ran, and what protecting it found;
// on hosts without locks, what watching it found.
type InVM struct {
	Mode string `json:"mode"` // On, Strict, Off, or Clone for a --clone VM
	// Chosen is set once the user chose the mode (--no-git-protect,
	// devvm git-protect), rather than the host: off chosen on a host
	// without locks quiets the launches' warning (WarnUnprotected).
	Chosen  bool    `json:"chosen,omitempty"`
	Next    string  `json:"next,omitempty"` // the mode chosen while the VM ran, set when it's next stopped
	Made    Made    `json:"made,omitzero"`
	Watched Watched `json:"watched,omitzero"`
}

// Made is what protecting a VM's .git found, and checks compare with.
type Made struct {
	// Protected is set once its protection was set up as a new VM's
	// (NewVM) or turned on: until then, there's nothing to compare with.
	Protected bool `json:"protected,omitempty"`
	// GitID is .git's identity (platform.FolderID) when it was last
	// locked.
	GitID string `json:"git_id,omitempty"`
	// GitDirs is the git dirs besides .git that it locked, relative to
	// the project folder: those under .git/modules and the submodules'
	// in the working tree.
	GitDirs []string `json:"git_dirs,omitempty"`
	// Delegations is a hash of what the locked files hand over to the
	// project folder.
	Delegations string `json:"delegations,omitempty"`
}

// Of returns a VM's .git protection section.
func Of(st *state.VM) (InVM, error) { return state.Section[InVM](st, Section) }

// Set sets a VM's .git protection section, in st alone: inside
// st.Update.
func Set(st *state.VM, g InVM) error { return state.SetSection(st, Section, g) }

// Mode returns a VM's .git protection: on, strict, off, or clone for a
// --clone VM, which mounts no .git; "" when it can't be read.
func Mode(st *state.VM) string {
	g, _ := Of(st)
	return g.Mode
}

// record changes the VM's section as f says, under its lock.
func record(ctx context.Context, vm *state.VM, f func(*InVM)) error {
	return vm.Update(ctx, func(vm *state.VM) error {
		g, err := Of(vm)
		if err != nil {
			return err
		}
		f(&g)
		return Set(vm, g)
	})
}

// maxDepth is how deep submodules of submodules are followed.
const maxDepth = 8

// Protector protects one project folder's .git.
type Protector struct {
	tree // the project folder (Dir) and the home folder for ~ (Home)
	// Locker locks, the host's way (HostLocker): on and strict need one
	// that does (Locker.Locks).
	Locker Locker
	// Reporter is where it warns.
	msg.Reporter
}

// New returns the protector for the project folder dir, ~ in git's paths
// being home (paths.Dirs.Home), locking the host's way (HostLocker),
// warning through r.
func New(dir, home string, r msg.Reporter) *Protector {
	return &Protector{tree: tree{Dir: dir, Home: home}, Locker: HostLocker(), Reporter: r}
}

// NewSection is the protection a new VM gets, recorded before its
// backend makes it: off when asked (chosen), or on hosts without locks,
// whose launches warn (WarnUnprotected); otherwise on, which NewVM sets
// up once it's made.
func (p *Protector) NewSection(off bool) InVM {
	switch {
	case off:
		return InVM{Mode: Off, Chosen: true}
	case !p.Locker.Locks():
		return InVM{Mode: Off}
	}
	return InVM{Mode: On}
}

// NewVM sets up the protection of a VM just made for the folder,
// stopped, as its mode says (NewSection): on locks .git and reports what
// a lock can't cover. On a host without locks, it records the watch's
// first sums instead, so the next check names what changed since.
func (p *Protector) NewVM(ctx context.Context, vm *state.VM) error {
	g, err := Of(vm)
	switch {
	case err != nil:
		return err
	case !p.Locker.Locks() && g.Mode != Clone:
		return watcher{p}.check(ctx, vm, g)
	case g.Mode != On && g.Mode != Strict || !p.Locker.Locks():
		return nil
	}
	made := p.protect(ctx, p.scan(ctx), g, whyNew, lockNow)
	return record(ctx, vm, func(g *InVM) { g.Made = made })
}

// Modes of .git protection, as devvm git-protect takes them. Strict is on,
// with host git status and diff kept out of submodules.
const (
	On     = "on"
	Strict = "strict"
	Off    = "off"
)

// Clone is the mode of a --clone VM, which mounts no .git to protect.
const Clone = "clone"

// Modes are the modes devvm git-protect takes.
var Modes = []string{On, Strict, Off}

// Next returns the mode chosen for a VM while it ran (SetNext), which
// the caller sets with Set once it's stopped; "" for none.
func Next(vm *state.VM) string {
	g, _ := Of(vm)
	return g.Next
}

// SetNext saves mode for a running VM, whose locks can't change until it
// stops: the caller sets it with Set then, and a later choice replaces it.
func SetNext(ctx context.Context, vm *state.VM, mode string) error {
	return record(ctx, vm, func(g *InVM) { g.Next = mode })
}

// Set changes the protection of a stopped VM: on and strict lock .git as
// it is now, as trusted, and report what a lock can't cover; strict also
// sets diff.ignoreSubmodules=all in the locked .git/config, which on and
// off take out again; off unlocks. On and strict need a host that locks.
// Once it's set, a mode saved for the VM's next stop (SetNext) is
// dropped: both are recorded at once, with the mode as the user's choice
// (Chosen).
func (p *Protector) Set(ctx context.Context, vm *state.VM, mode string) error {
	g, err := Of(vm)
	if err != nil {
		return err
	}
	was := g.Mode
	if mode == Off {
		p.Unlock(ctx, vm)
		if was == Strict {
			if err := p.ignoreSubmodules(ctx, false); err != nil {
				return err
			}
		}
		return record(ctx, vm, func(g *InVM) { g.Mode, g.Chosen, g.Next = Off, true, "" })
	}
	if !p.Locker.Locks() {
		return msg.GitProtectOnNeedsMacos()
	}
	if mode == Strict || was == Strict {
		if err := p.ignoreSubmodules(ctx, mode == Strict); err != nil {
			// Locked as it was.
			if cfg := filepath.Join(p.git(), "config"); was != Off && isFile(cfg) && p.realInDir(cfg) {
				p.flag(ctx, true, cfg)
			}
			return err
		}
	}
	g.Mode = mode
	made := p.protect(ctx, p.scan(ctx), g, whyOn, lockNow)
	return record(ctx, vm, func(g *InVM) { g.Mode, g.Chosen, g.Next, g.Made = mode, true, "", made })
}

// ignoreSubmodules sets diff.ignoreSubmodules=all in .git/config, or takes
// it out, unlocking the file for the moment (protect locks it again).
func (p *Protector) ignoreSubmodules(ctx context.Context, on bool) error {
	cfg := filepath.Join(p.git(), "config")
	if !isFile(cfg) || !p.realInDir(cfg) {
		if on && exists(p.git()) {
			return msg.GitConfigFailed(cfg)
		}
		return nil
	}
	if !p.flag(ctx, false, cfg) {
		if on {
			return msg.GitConfigFailed(cfg)
		}
		return nil
	}
	args := []string{"config", "--file", cfg, "--unset", "diff.ignoreSubmodules"}
	if on {
		args = []string{"config", "--file", cfg, "diff.ignoreSubmodules", "all"}
	}
	// --unset fails when there's nothing to take out.
	if err := hostgit.Command(ctx, args...).Run(); err != nil && on {
		return msg.GitConfigFailed(cfg)
	}
	return nil
}

// Check runs on every launch of an existing VM: it locks again what's new
// (a submodule, a repo made since), and reports what a lock can't stop,
// once each: a .git replaced since it was locked (then nothing else), a
// repo that appeared, submodule git dirs that appeared, hooks in a hooks
// folder that wasn't locked, and includes or hook links into the project
// folder when they change. Locks are set only while the VM is stopped
// (running), so nothing in it can race them; a running VM gets them at
// its next start. A VM whose protection was never set up (its creation
// stopped short, or devvm adopt moved it) gets it as a new VM does. On a
// host without locks, a stopped VM's .git is watched instead, whatever
// its mode: what a lock can't stop is reported all the same, and what a
// lock would cover that changed since the last check is named
// (watcher.check).
func (p *Protector) Check(ctx context.Context, vm *state.VM, running bool) error {
	g, err := Of(vm)
	switch {
	case err != nil:
		return err
	case !p.Locker.Locks():
		if running || g.Mode == Clone {
			return nil
		}
		return watcher{p}.check(ctx, vm, g)
	case g.Mode != On && g.Mode != Strict:
		return nil
	case !g.Made.Protected:
		if running {
			return nil
		}
		return p.NewVM(ctx, vm)
	}
	h := lockNow
	if running {
		h = later
	}
	made := p.protect(ctx, p.scan(ctx), g, whyCheck, h)
	return record(ctx, vm, func(g *InVM) { g.Made = made })
}

// Unlock takes the locks off, for a VM that's being deleted or turned
// off: what's to lock now, and the hooks and config of every git dir its
// state records locking (vm; nil for none), which the VM may since have
// dropped from its index. Like locking, it never acts through a symlink.
func (p *Protector) Unlock(ctx context.Context, vm *state.VM) {
	if !p.Locker.Locks() {
		return
	}
	todo := p.scan(ctx).paths
	if vm != nil {
		g, _ := Of(vm)
		for _, d := range p.abs(g.Made.GitDirs) {
			todo = append(todo, filepath.Join(d, "hooks"), filepath.Join(d, "config"))
		}
	}
	done := map[string]bool{}
	for _, path := range todo {
		if done[path] {
			continue
		}
		done[path] = true
		if _, err := os.Lstat(path); err == nil && p.realInDir(path) {
			p.flag(ctx, false, path)
		}
	}
}

// lock locks the paths to protect (s.paths), which only happens while
// the VM is stopped, so nothing in it can race the checks.
func (p *Protector) lock(ctx context.Context, s snapshot) {
	for _, path := range s.paths {
		if !p.realInDir(path) {
			p.Warn(msg.GitNotLocked(path))
			continue
		}
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			// So the VM can't create it. O_EXCL and mkdir fail on any
			// existing name, and neither follows a symlink.
			switch filepath.Base(path) {
			case "config.worktree":
				if f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil { //nolint:gosec,forbidigo // a new empty file, checked by realInDir, never read
					_ = f.Close()
				}
			case "hooks":
				_ = os.Mkdir(path, 0o700)
			}
		}
		if _, err := os.Lstat(path); err == nil {
			p.flag(ctx, true, path)
		}
	}
}

// flag locks a path realInDir passed (lock), or unlocks it, then checks
// it again. Between the check and the locker, a running VM could swap a
// folder above the path for a symlink to somewhere else: a path that
// moved meanwhile is unlocked again where it landed, and reported.
func (p *Protector) flag(ctx context.Context, lock bool, path string) bool {
	before := devIno(path)
	if before == "" {
		return true
	}
	if lock {
		_ = p.Locker.Lock(ctx, path)
	} else {
		_ = p.Locker.Unlock(ctx, path)
	}
	if devIno(path) == before && p.realInDir(path) {
		return true
	}
	if lock {
		_ = p.Locker.Unlock(ctx, path)
	}
	p.Warn(msg.GitLockRaced(path))
	return false
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func isDir(path string) bool { fi, err := os.Stat(path); return err == nil && fi.IsDir() }

func isLink(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode()&fs.ModeSymlink != 0
}

func isFile(path string) bool { fi, err := os.Stat(path); return err == nil && fi.Mode().IsRegular() }

// isHooks reports whether path is a hooks folder, itself, not a symlink.
func isHooks(path string) bool {
	return filepath.Base(path) == "hooks" && isDir(path) && !isLink(path)
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
		return nil // unreadable: skipped, like find
	})
	return found
}

// git runs git on the host (hostgit) and returns its output; nothing when
// it fails.
func git(ctx context.Context, args ...string) []byte {
	out, err := hostgit.Output(ctx, args...)
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

// devIno is what's at path (not following a symlink) as dev:ino, or "".
func devIno(path string) string {
	fi, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	return platform.DevIno(fi)
}

// folderID is what's at path as platform.FolderID has it, or "".
func folderID(path string) string {
	id, _ := platform.FolderID(path)
	return id
}
