package vm

import (
	"context"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/state"
)

// vmFolders finds and deletes VMs' folders in devvm's data folder: a
// folder's VM by its path's hash, and every file a VM had when it goes.
type vmFolders struct{ *Env }

// teardown deletes what devvm keeps for a VM its backend no longer has (t):
// deleted by devvm rm, by an interrupted creation, or some other way. It's
// the one place a VM's files go, in order: what runs for it (its proxy,
// its port forwarder, its link session), then what's
// outside its folder (its clone's remote or .git's locks, other VMs'
// links to it and its group, its host keys, with leaves its backend's own
// leftovers, backend.Forgetter), then its folder, every file of its in
// it, and last its ssh config entry. leaves is false after the backend
// deleted the VM itself, which took its leftovers with it.
func (e vmFolders) teardown(ctx context.Context, t *found, leaves bool) error {
	vm := t.f.Name
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	linkRuns(e).stop(ctx, vm)
	if isClone(t.st) {
		clones(e).removeRemote(ctx, t.f.Dir, vm, sectionOf[cloneInVM](t.st, cloneSection).Made.Remote)
	} else if m := gitprotect.Mode(t.st); m == gitprotect.On || m == gitprotect.Strict {
		e.Protector(t.f.Dir).Unlock(ctx, t.st)
	}
	if err := Groups(e).forget(ctx, vm); err != nil {
		return err
	}
	if err := e.sshConfig().ForgetHostKeys(vm); err != nil {
		return err
	}
	if f, ok := t.b.(backend.Forgetter); ok && leaves {
		if err := f.Forget(ctx, vm); err != nil {
			return err
		}
	}
	if err := state.RemoveFolder(ctx, e.Dirs, vm); err != nil {
		return err
	}
	return e.sshConfig().Write(ctx)
}

// clear deletes what a VM made for this folder before left in its
// folder, which a new VM by that name replaces: its state, its files, and
// what still runs for it (its proxy, its port forwarder, its link
// session).
func (e vmFolders) clear(ctx context.Context, vm string) error {
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	linkRuns(e).stop(ctx, vm)
	return state.RemoveFolder(ctx, e.Dirs, vm)
}
