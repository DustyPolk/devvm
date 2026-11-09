package vm

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/shell"
	"github.com/devvmhq/devvm/internal/state"
)

// mountInVM is a VM's mount section: the cache mode of its project
// folder's mount (backend.CacheModes), which a new VM gets as it's made,
// and devvm mount-cache sets. "" for a VM whose mode wasn't chosen, which
// keeps its backend's own: one made before devvm chose it, whose mount
// its starts leave as it is, warning at each launch where the backend's
// mode can lose writes (warnMountCache).
type mountInVM struct {
	Cache string `json:"cache,omitempty"`
}

// cacheOf is the cache mode chosen for st's VM, "" for none. A stored
// mmap, which a build of devvm before it settled on none alone could
// write, counts as none chosen, so the VM warns and devvm mount-cache none
// still switches it.
func cacheOf(st *state.VM) string {
	c := sectionOf[mountInVM](st, mountSection).Cache
	if c == backend.CacheMmap {
		return ""
	}
	return c
}

// CheckCacheMode refuses a cache mode devvm doesn't set
// (backend.CacheModes), as devvm mount-cache or a VM's state gives it.
func CheckCacheMode(v string) error {
	if !slices.Contains(backend.CacheModes(), v) {
		return msg.MountCacheInvalid(strconv.Quote(v))
	}
	return nil
}

// readMount is st's mount section, refused when its cache mode isn't one,
// naming the VM's state file; a stored mmap is read as none chosen
// (cacheOf).
func readMount(st *state.VM) (mountInVM, error) {
	m, err := state.Section[mountInVM](st, mountSection)
	if err != nil {
		return m, err
	}
	if m.Cache != "" && m.Cache != backend.CacheMmap {
		if err := CheckCacheMode(m.Cache); err != nil {
			mm, _ := errors.AsType[msg.M](err)
			return m, &state.CorruptError{Path: st.File(), Reason: mountSection + ".cache: ", Msg: mm}
		}
	}
	return m, nil
}

// newCache is the cache mode a new VM on b gets, or one whose folder
// devvm clone off mounts anew: none, where b has the choice; "" where it
// has none to make.
func (e *Env) newCache(b backend.Backend) string {
	if !e.Registry.Allowed(b, backend.FeatureMountCache) {
		return ""
	}
	return backend.CacheNone
}

// warnMountCache warns at each launch of a VM, as in has it, whose
// project folder's mount keeps a cache mode no one chose that can lose
// writes (its backend's own, such as Lima's mmap on a Linux host), until
// devvm mount-cache none chooses one. A --clone VM mounts no folder.
func (e *Env) warnMountCache(st *state.VM, in backend.Instance) {
	if isClone(st) || cacheOf(st) != "" || in.ProjectCache == "" || in.ProjectCache == backend.CacheNone {
		return
	}
	e.Warn(msg.MountCacheUnchosen(in.ProjectCache, st.Name()))
}

// checkMounted refuses a launch into t's VM, running, whose project folder
// isn't a mount point in the guest, where its mount has a cache mode (a
// Linux host's 9p): one a remount or the guest left unmounted, where
// what's written goes to the VM's own disk and vanishes once the folder
// is mounted again. It costs one command over the VM's ssh, which answers
// in words rather than mountpoint's exit code (util-linux's 32, busybox's
// 1): a check that fails to run, or gives no answer, refuses too, as
// project_unchecked. Its connection carries agent, the agent the VM runs
// with, as devvm's every connection to it does.
func (s starter) checkMounted(ctx context.Context, t *target, agent string) error {
	if isClone(t.st) || t.in.ProjectCache == "" || t.in.Project == "" {
		return nil
	}
	out := &smallOutput{}
	script := "if mountpoint -q " + shell.Quote(t.in.Project) + "; then echo " + mountedWord + "; else echo " + unmountedWord + "; fi"
	err := t.b.Exec(ctx, t.f.Name, []string{"sh", "-c", script}, backend.ExecOpts{Agent: agent, Stdout: out})
	switch answer := strings.TrimSpace(out.String()); {
	case ctx.Err() != nil:
		return ctx.Err()
	case err == nil && answer == mountedWord:
		return nil
	case err == nil && answer == unmountedWord:
		return msg.ProjectUnmounted(t.f.Name)
	case err == nil:
		err = errors.New("the guest gave no answer")
	}
	return msg.ProjectUnchecked(t.f.Name).Because(err)
}

// The words checkMounted's guest command answers with.
const (
	mountedWord   = "devvm-mounted"
	unmountedWord = "devvm-not-mounted"
)

// smallOutput keeps the first 64 bytes a guest command writes: enough for
// a word, and no more of what a VM could write.
type smallOutput struct{ buf []byte }

func (o *smallOutput) Write(p []byte) (int, error) {
	o.buf = append(o.buf, p[:min(len(p), 64-len(o.buf))]...)
	return len(p), nil
}

func (o *smallOutput) String() string { return string(o.buf) }

// MountCacheShown is a VM's cache mode as devvm mount-cache shows it.
type MountCacheShown struct {
	// Mode is the mode its project folder's mount has; a --clone VM's is
	// the one it gets once it mounts the folder.
	Mode string
	// Chosen: the mode was chosen, as the VM was made or with devvm
	// mount-cache, rather than kept from its backend.
	Chosen bool
	// Pending is the mode its next start gives it; "" for none.
	Pending string
}

// MountCache returns how the folder's VM caches its project folder's
// mount, saying what changes at its next start, and warning as each
// launch does of a mode no one chose (warnMountCache).
func (e Settings) MountCache(ctx context.Context) (MountCacheShown, error) {
	t, err := e.finder.target(ctx, need{has: hasMade, noVM: msg.NoVMMountCache})
	if err != nil {
		return MountCacheShown{}, err
	}
	vm, st := t.f.Name, t.st
	if err := e.cacheable(t); err != nil {
		return MountCacheShown{}, err
	}
	chosen := cacheOf(st)
	s := MountCacheShown{Mode: t.in.ProjectCache, Chosen: chosen != ""}
	switch {
	case isClone(st):
		s.Mode = cmp.Or(chosen, backend.CacheNone)
		e.Note(msg.MountCacheClone(s.Mode, vm))
	case chosen != "" && chosen != t.in.ProjectCache:
		s.Pending = chosen
		e.Note(msg.MountCachePending(chosen, vm))
	default:
		e.warnMountCache(st, t.in)
	}
	return s, nil
}

// cacheable refuses t's VM a cache mode its backend has no choice of
// (Require), or that its project folder's mount has none of, as its
// instance says: a mode saved for it would never apply. A --clone VM
// mounts no folder yet, and keeps one for when it does.
func (e Settings) cacheable(t *target) error {
	if err := e.Registry.Require(t.b, t.f.Name, backend.FeatureMountCache); err != nil {
		return err
	}
	if !isClone(t.st) && t.in.ProjectCache == "" {
		return msg.MountCacheNoChoice(t.f.Name)
	}
	return nil
}

// SetMountCache chooses how the folder's VM caches its project folder's
// mount, none being the one mode devvm sets: from its next start when
// it's stopped; when it runs, after a restart the user agrees to
// (restartOr), else from its next start. A --clone VM keeps it for when
// it mounts the folder (devvm clone off).
func (e Settings) SetMountCache(ctx context.Context, mode string) error {
	if err := CheckCacheMode(mode); err != nil {
		return err
	}
	t, err := e.finder.target(ctx, need{has: hasMade, noVM: msg.NoVMMountCache})
	if err != nil {
		return err
	}
	vm, st, in := t.f.Name, t.st, t.in
	if err := e.cacheable(t); err != nil {
		return err
	}
	if err := update(ctx, st, mountSection, func(m *mountInVM) error { m.Cache = mode; return nil }); err != nil {
		return err
	}
	switch {
	case isClone(st):
		e.Info(msg.MountCacheClone(mode, vm))
	case in.ProjectCache == mode:
		e.Info(msg.VMMountCache(mode, vm))
	case in.Status == backend.StatusRunning:
		return e.restartOr(ctx, vm, e.Warn, msg.NextStart(1, vm))
	default:
		e.Info(msg.MountCachePending(mode, vm))
	}
	return nil
}
