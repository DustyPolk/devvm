package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Lifecycle stops the folder's VM, gives it to the folder, exports its
// agents' history and deletes it.
type Lifecycle struct{ *Env }

// Stop stops the folder's VM, its proxy and its link session. Then, the
// VM being stopped, the .git protection chosen while it ran is set, .git
// is locked again and checked (on Linux, set-id files too) as on a
// launch that finds it stopped (checkGit), and its ssh config entry goes.
func (e Lifecycle) Stop(ctx context.Context) error {
	t, err := e.folderVM(ctx, msg.NoVM)
	if err != nil {
		return err
	}
	vm := t.f.Name
	in, err := t.b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if in.Status == backend.StatusStopped {
		e.Info(msg.AlreadyStopped(vm))
	} else {
		if err := t.b.Stop(ctx, vm); err != nil {
			return err
		}
		e.Info(msg.Stopped(vm))
	}
	e.Proxy.Stop(vm)
	linkRuns(e).stop(ctx, vm)
	if err := checkGit(ctx, e.Env, t, false); err != nil {
		return err
	}
	return e.sshConfig().Write(ctx)
}

// Adopt gives the folder the VM made for an earlier folder at its path,
// after asking in a terminal, or with force (--force): its files, tokens
// and allowlist. Its .git is locked as new at its next launch. Only the
// VM named for this path is adopted, never a parent folder's.
func (e Lifecycle) Adopt(ctx context.Context, force bool) error {
	t, _, err := e.lookup(ctx, false)
	if err != nil {
		return err
	}
	if !t.exists {
		return msg.NoVM(t.f.Dir)
	}
	vm, st := t.f.Name, t.st
	if e.sameFolder(t.f, st) {
		e.Info(msg.VMAlreadyHere(vm))
		return nil
	}
	was := st.Dir
	switch {
	case force:
	case !e.Terminal():
		return msg.VMOtherFolderAdopt(vm, was)
	case !e.Ask(msg.AdoptVM(vm, was)):
		return nil
	}
	id, err := platform.FolderID(t.f.Dir)
	if err != nil {
		return err
	}
	// Its folder, and .git's protection set up as a new VM's at its next
	// launch, at once.
	err = st.Update(ctx, func(st *state.VM) error {
		st.Dir, st.Folder = t.f.Dir, id
		g, err := gitprotect.Of(st)
		if err != nil {
			return err
		}
		g.Made = gitprotect.Made{}
		return gitprotect.Set(st, g)
	})
	if err != nil {
		return err
	}
	e.Info(msg.VMAdopted(t.f.Dir, vm))
	return nil
}

// RmOptions are devvm rm's flags.
type RmOptions struct {
	Force    bool // --force: no "delete it?" asked, which needs a terminal otherwise
	NoExport bool // --no-export: the agents' history isn't exported first
	// NoCloneCheck (--no-clone-check): a --clone VM's clone isn't checked
	// for work the repo lacks first, which is lost.
	NoCloneCheck bool
}

// Rm deletes the folder's VM, after asking in a terminal, or with
// --force; without either it refuses (force_needed). Its agents' history
// is exported first unless --no-export: an export that fails keeps it.
// A --clone VM's branches the repo lacks and uncommitted changes are
// checked first (cloneRmCheck), which asks only in a terminal, unless
// --no-clone-check, which warns that they're lost instead. A VM made
// for an earlier folder at this path, or a parent folder's VM found from
// a subfolder, is deleted only after asking in a terminal, which --force
// never answers, and never without one; the earlier folder's isn't
// exported, as its folder is gone.
func (e Lifecycle) Rm(ctx context.Context, o RmOptions) error {
	t, err := e.findExisting(ctx)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	asked := false
	if t.sub != "" {
		if !e.Terminal() {
			return msg.RmParentVM(t.f.Dir, vm)
		}
		if !e.Ask(msg.DeleteParentVM(t.f.Dir, vm)) {
			return nil
		}
		asked = true
	}
	if !e.sameFolder(t.f, st) {
		was := st.Dir
		if !e.Terminal() {
			return msg.VMOtherFolderRm(vm, was)
		}
		if !e.Ask(msg.DeleteOtherFolderVM(vm, was)) {
			return nil
		}
		asked, o.NoExport = true, true
	}
	confirm := !asked && !o.Force
	if confirm && !e.Terminal() {
		return msg.ForceNeeded("devvm rm")
	}
	// A VM whose creation didn't finish never ran: no history, no work.
	checkClone := st.Finished() && isClone(st) && e.sameFolder(t.f, st)
	if checkClone && o.NoCloneCheck {
		e.Warn(msg.CloneCheckSkipped(vm))
		checkClone = false
	}
	if confirm && !e.Ask(msg.RmConfirm(vm)) {
		e.Info(msg.Kept(vm))
		return nil
	}
	if checkClone {
		keep, err := clones(e).cloneRmCheck(ctx, t)
		if err != nil || keep {
			return err
		}
	}
	if !o.NoExport && st.Finished() && len(image.AgentsOf(tools.Names(st))) > 0 {
		if err := e.export(ctx, t, "", msg.RmExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// RmVM deletes the VM that name names, by its name or its ssh alias, when
// it's this folder's (Rm) or its project folder is gone (FolderGone); a
// VM whose folder is still there is deleted from that folder. It asks
// first in a terminal, or deletes with --force, and refuses without
// either (force_needed), as Rm does. Its agents' history is exported
// first unless --no-export, which needs the VM running: one stopped would
// make its folder again as it starts (its backend makes a missing mount's
// folder), so it's refused instead, unless it's a --clone VM, which
// mounts none. A --clone VM's work can't be fetched into a repo that's
// gone, so it's deleted only with --no-clone-check.
func (e Lifecycle) RmVM(ctx context.Context, name string, o RmOptions) error {
	all, err := recordedVMs(e.Dirs)
	if err != nil {
		return err
	}
	vm := project.Named(name, all)
	if vm == "" {
		return msg.RmUnknownVM(name)
	}
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		return err
	}
	if st.Dir == e.Dir {
		return e.Rm(ctx, o)
	}
	if !FolderGone(st) {
		return msg.RmVMFolderThere(st.Dir, vm)
	}
	t, err := e.inspect(ctx, project.Folder{Dir: st.Dir, Name: vm}, st)
	if err != nil {
		return err
	}
	if !t.exists {
		return msg.RmVMLost(vm)
	}
	return e.rmGone(ctx, t, o)
}

// rmGone deletes t, a VM whose folder is gone, for RmVM.
func (e Lifecycle) rmGone(ctx context.Context, t *found, o RmOptions) error {
	vm, st := t.f.Name, t.st
	clone := st.Finished() && isClone(st)
	if clone && !o.NoCloneCheck {
		return msg.RmGoneClone(st.Dir, vm)
	}
	export := !o.NoExport && st.Finished() && len(image.AgentsOf(tools.Names(st))) > 0
	if err := e.stillGone(ctx, t, export && !clone); err != nil {
		return err
	}
	if !o.Force && !e.Terminal() {
		return msg.ForceNeeded("devvm rm")
	}
	if clone {
		e.Warn(msg.CloneCheckSkipped(vm))
	}
	if !o.Force && !e.Ask(msg.DeleteGoneFolderVM(st.Dir, vm)) {
		e.Info(msg.Kept(vm))
		return nil
	}
	// While it asked, the folder may have come back (a disk plugged in
	// again), another devvm may have made the VM again there, or stopped
	// it, which an export would start, making its folder again.
	if err := e.stillGone(ctx, t, export && !clone); err != nil {
		return err
	}
	if export {
		if err := e.export(ctx, t, "", msg.RmVMExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// stillGone checks, for rmGone, that t's state is still the one it read
// (its ID) and its folder still gone, and with running that the VM still
// runs, so its export doesn't start it.
func (e Lifecycle) stillGone(ctx context.Context, t *found, running bool) error {
	vm := t.f.Name
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		return err
	}
	if st.ID != t.st.ID || !FolderGone(st) {
		return msg.RmVMFolderThere(t.st.Dir, vm)
	}
	if !running {
		return nil
	}
	in, err := t.b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if in.Status != backend.StatusRunning {
		return msg.RmGoneStopped(t.st.Dir, vm)
	}
	return nil
}

// Export saves the folder's VM's agents' history to file (absolute), or
// to the exports folder when it's empty, starting the VM if it's stopped.
func (e Lifecycle) Export(ctx context.Context, file string) error {
	t, err := e.existing(ctx)
	if err != nil {
		return err
	}
	return e.export(ctx, t, file, backend.Logged(msg.HistoryNotExported(t.f.Name)))
}

// exportFile is a new export's file in the exports folder.
func (e Lifecycle) exportFile(vm string) string {
	return filepath.Join(e.Dirs.Exports(), vm+"-"+time.Now().Format("20060102-150405")+".tar.gz")
}

// export saves the VM's agents' history (transcripts, sessions) to a
// tarball at file, or a new one in the exports folder when it's "", with
// a file beside it naming the VM's folder (paths.ExportFolder), for devvm
// history to list once the VM is gone; starting the VM if it's stopped; failed is the error
// when its tar fails. Their logins aren't in it. It's written next to
// file and renamed into place once complete, so a failed export leaves
// nothing and replaces nothing. The file is the user's alone (600), and
// so is the exports folder, whatever mode it had; a folder of the user's
// keeps its own.
func (e Lifecycle) export(ctx context.Context, t *found, file string, failed error) error {
	vm, b := t.f.Name, t.b
	own := file == ""
	if own {
		file = e.exportFile(vm)
	}
	if err := e.ensureRunning(ctx, t); err != nil {
		return err
	}
	// devvm's exports folder is the user's alone; a folder the user named
	// keeps its mode.
	if dir := filepath.Dir(file); dir == e.Dirs.Exports() {
		if err := state.PrivateDir(dir); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(file), ".devvm-export-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	done := e.AddCleanup(func(context.Context) { _ = os.Remove(tmp) })
	defer func() {
		_ = os.Remove(tmp)
		done()
	}()
	excludes := make([]string, len(historyExcludes))
	for i, p := range historyExcludes {
		excludes[i] = "--exclude=" + p
	}
	script := "cd ~ && ls -d " + strings.Join(historyPaths, " ") + " 2>/dev/null | tar -czf - " + strings.Join(excludes, " ") + " -T -"
	err = b.Exec(ctx, vm, []string{"bash", "-c", script}, backend.ExecOpts{Stdout: out, Stderr: e.LogWriter()})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failed
	}
	if err := os.Rename(tmp, file); err != nil {
		return err
	}
	if own {
		if err := state.WriteFile(paths.ExportFolder(file), []byte(t.f.Dir+"\n")); err != nil {
			return err
		}
	}
	e.Info(msg.Exported(file, vm))
	return nil
}

// delete deletes the VM, then what devvm keeps for it (teardown): its
// folder, its proxy and link session, .git's locks or the repo's remote
// for its clone, other VMs' links to it, its host keys and its ssh config
// entry. A delete that failed but left no VM (it was half gone already)
// goes on like one that worked.
func (e Lifecycle) delete(ctx context.Context, t *found) error {
	vm := t.f.Name
	if err := t.b.Delete(ctx, vm); err != nil {
		if _, ierr := t.b.Inspect(ctx, vm); !errors.Is(ierr, backend.ErrNotFound) {
			return err
		}
	}
	e.Info(msg.Deleted(vm))
	return vmFolders(e).teardown(ctx, t, false)
}
