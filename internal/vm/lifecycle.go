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
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Lifecycle stops the folder's VM, gives it to the folder, exports its
// agents' history, makes it again (remake.go) and deletes it.
type Lifecycle struct{ *Env }

// Stop stops the folder's VM, its proxy and its link session. Then, the
// VM being stopped, the .git protection chosen while it ran is set, .git
// is locked again and checked (on Linux, set-id files too) as on a
// launch that finds it stopped (starter.hostChecks), and its ssh config entry goes.
func (e Lifecycle) Stop(ctx context.Context) error {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm := t.f.Name
	if t.in.Status == backend.StatusStopped {
		e.Info(msg.AlreadyStopped(vm))
	} else {
		if err := t.stop(ctx); err != nil {
			return err
		}
		e.Info(msg.Stopped(vm))
	}
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	e.linkRuns.stop(ctx, vm)
	if err := e.starter.hostChecks(ctx, t, false, false); err != nil {
		return err
	}
	return e.writeSSHConfig(ctx)
}

// Adopt gives the folder the VM made for an earlier folder at its path,
// after asking in a terminal, or with force (--force): its files, tokens
// and allowlist. Its .git is locked as new at its next launch. Only the
// VM of this path is adopted, never a parent folder's. A VM whose folder
// moved here is moved here (finder.moveHere), --force answering its
// question too. With name, it's that VM (finder.adoptVM).
func (e Lifecycle) Adopt(ctx context.Context, name string, force bool) error {
	if name != "" {
		return e.finder.adoptVM(ctx, name, force)
	}
	mv := moveAsk
	if force {
		mv = moveForce
	}
	t, err := e.finder.target(ctx, need{own: true, has: hasMachine, move: mv})
	if err != nil || t.moved {
		return err
	}
	vm, st := t.f.Name, t.st
	if !t.other {
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
// --force; without either it refuses (force_needed). One in the middle of
// a remake is deleted too, its old machine with it (teardown). Its agents' history
// is exported first unless --no-export: an export that fails keeps it.
// A --clone VM's branches the repo lacks and uncommitted changes are
// checked first (cloneCheck), which asks only in a terminal, unless
// --no-clone-check, which warns that they're lost instead. A VM made
// for an earlier folder at this path, or a parent folder's VM found from
// a subfolder, is deleted only after asking in a terminal, which --force
// never answers, and never without one; the earlier folder's isn't
// exported, as its folder is gone. While another devvm makes the folder's
// VM it refuses (holdForRm).
func (e Lifecycle) Rm(ctx context.Context, o RmOptions) error {
	t, err := e.finder.target(ctx, need{has: hasVM})
	if err != nil {
		return err
	}
	// Never one another devvm is making.
	release, err := holdForRm(e.Env, t)
	if err != nil {
		return err
	}
	defer release()
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
	if t.other {
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
	checkClone := st.Finished() && isClone(st) && !t.other
	if checkClone && o.NoCloneCheck {
		e.Warn(msg.CloneCheckSkipped("devvm rm", vm))
		checkClone = false
	}
	if confirm && !e.Ask(msg.RmConfirm(vm)) {
		e.Info(msg.Kept(vm))
		return nil
	}
	if checkClone {
		keep, err := cloneCheck{clones: e.clones, command: "devvm rm"}.run(ctx, t)
		if err != nil || keep {
			return err
		}
	}
	// A remake that didn't finish may have left no machine by its name:
	// its history was exported then, unless --no-export.
	if !o.NoExport && t.exists && st.Finished() && len(image.AgentsOf(tools.Names(st))) > 0 {
		if err := e.export(ctx, t, "", msg.RmExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// RmVM deletes the VM that name names, by its name or its ssh alias, when
// it's this folder's (Rm) or its project folder is gone or not connected
// (FolderOf); a
// VM whose folder is still there is deleted from that folder. It asks
// first in a terminal, or deletes with --force, and refuses without
// either (force_needed), as Rm does. Its agents' history is exported
// first unless --no-export, which needs the VM running: one stopped would
// make its folder again as it starts (its backend makes a missing mount's
// folder), so it's refused instead, unless it's a --clone VM, which
// mounts none. A --clone VM's work can't be fetched into a repo that's
// gone, so it's deleted only with --no-clone-check. A VM that records
// this folder but isn't this folder's own (a vm.json changed by hand) is
// one whose folder is there, refused.
func (e Lifecycle) RmVM(ctx context.Context, name string, o RmOptions) error {
	t, err := e.finder.target(ctx, need{name: name})
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	// This folder's own VM, when it's the VM named: never another VM
	// that records this folder.
	if here, err := e.finder.dir(); err == nil && st.Dir == here {
		own, err := e.finder.folderVM(here)
		if err != nil {
			return err
		}
		if own == vm {
			return e.Rm(ctx, o)
		}
	}
	folder := FolderOf(st)
	if folder == FolderThere {
		return msg.RmVMFolderThere(st.Dir, vm)
	}
	if !t.exists {
		return msg.RmVMLost(vm)
	}
	return e.rmGone(ctx, t, folder, o)
}

// rmGone deletes t, a VM whose folder is gone or not connected, for RmVM.
// One whose folder is on a disk that isn't connected is deleted only
// after asking in a terminal, which --force never answers.
func (e Lifecycle) rmGone(ctx context.Context, t *target, folder FolderState, o RmOptions) error {
	release, err := holdForRm(e.Env, t)
	if err != nil {
		return err
	}
	defer release()
	vm, st := t.f.Name, t.st
	clone := st.Finished() && isClone(st)
	if clone && !o.NoCloneCheck {
		return msg.RmGoneClone(st.Dir, vm)
	}
	export := !o.NoExport && st.Finished() && len(image.AgentsOf(tools.Names(st))) > 0
	if err := e.stillGone(ctx, t, folder, export && !clone); err != nil {
		return err
	}
	unconnected := folder == FolderNotConnected
	if unconnected && !e.Terminal() {
		return msg.RmVMNotConnected(st.Dir, vm)
	}
	if !o.Force && !e.Terminal() {
		return msg.ForceNeeded("devvm rm")
	}
	if clone {
		e.Warn(msg.CloneCheckSkipped("devvm rm", vm))
	}
	if !e.askRmGone(t, unconnected, o.Force) {
		e.Info(msg.Kept(vm))
		return nil
	}
	// While it asked, the folder may have come back (a disk plugged in
	// again), another devvm may have made the VM again there, or stopped
	// it, which an export would start, making its folder again.
	if err := e.stillGone(ctx, t, folder, export && !clone); err != nil {
		return err
	}
	if export {
		if err := e.export(ctx, t, "", msg.RmVMExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// askRmGone asks, for rmGone, whether to delete t: with force, only when
// its folder is on a disk that isn't connected (unconnected), which
// --force never answers.
func (e Lifecycle) askRmGone(t *target, unconnected, force bool) bool {
	if unconnected {
		return e.Ask(msg.DeleteNotConnectedFolderVM(t.st.Dir, t.f.Name))
	}
	return force || e.Ask(msg.DeleteGoneFolderVM(t.st.Dir, t.f.Name))
}

// stillGone checks, for rmGone, that t's state is still the one it read
// (its ID) and its folder still in the state it found (folder), and with
// running that the VM still runs, so its export doesn't start it.
func (e Lifecycle) stillGone(ctx context.Context, t *target, folder FolderState, running bool) error {
	vm := t.f.Name
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		return err
	}
	if st.ID != t.st.ID || FolderOf(st) != folder {
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
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	return e.export(ctx, t, file, backend.Logged(msg.HistoryNotExported(t.f.Name)))
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
func (e Lifecycle) export(ctx context.Context, t *target, file string, failed error) error {
	vm := t.f.Name
	x := Exports{Dirs: e.Dirs}
	own := file == ""
	if own {
		file = x.newFile(vm, time.Now())
	}
	if err := t.ensureRunning(ctx); err != nil {
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
	err = t.exec(ctx, []string{"bash", "-c", script}, backend.ExecOpts{Stdout: out, Stderr: e.LogWriter()})
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
		if err := x.writeFolder(file, t.f.Dir); err != nil {
			return err
		}
	}
	e.Info(msg.Exported(file, vm))
	return nil
}

// Import puts an export of agents' history into the folder's VM: file
// (absolute), or the newest export devvm saved for the VM's folder when
// it's "", never another folder's. devvm starts a stopped VM for it.
func (e Lifecycle) Import(ctx context.Context, file string) error {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	if file == "" {
		x, ok, err := (Exports{Dirs: e.Dirs}).Newest(t.f.Dir)
		if err != nil {
			return err
		}
		if !ok {
			return msg.HistoryNoExport(t.f.Dir)
		}
		file = x.Path
	} else if _, err := os.Stat(file); err != nil {
		return err
	}
	if err := t.ensureRunning(ctx); err != nil {
		return err
	}
	return importHistory(ctx, e.Env, t, file)
}

// importHistory puts the export of agents' history at file into t's VM,
// which is running. The file is streamed in as it is, and the VM's tar
// reads it (historyImport): it comes from a VM, so the host never reads
// what's in it, and only a VM's own home gets it.
func importHistory(ctx context.Context, e *Env, t *target, file string) error {
	vm := t.f.Name
	f, err := os.Open(file) //nolint:gosec // the export the user named, or the one devvm saved for this folder
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	out := &capped{max: len(historyUnsafe) + 1}
	err = t.exec(ctx, []string{"bash", "-c", historyImport()}, backend.ExecOpts{Stdin: f, Stdout: out, Stderr: e.LogWriter()})
	switch {
	case err == nil:
	case ctx.Err() != nil:
		return ctx.Err()
	case strings.TrimSpace(out.String()) == historyUnsafe:
		return msg.HistoryImportUnsafe(file)
	default:
		return backend.Logged(msg.HistoryNotImported(file, vm))
	}
	e.Info(msg.Imported(file, vm))
	return nil
}

// offerHistory returns the export of agents' history the new VM vm, of
// the folder dir, gets: the newest devvm saved for dir, never another
// folder's, when the VM has an agent among its shared tools (names) and
// the user says yes in a terminal ([y/N]: the export is an old VM's
// content). Without a question it says how to import it later, and
// returns "".
func (e creation) offerHistory(vm, dir string, names []string) string {
	if len(image.AgentsOf(names)) == 0 {
		return ""
	}
	x, ok, err := (Exports{Dirs: e.Dirs}).Newest(dir)
	if err != nil || !ok {
		return ""
	}
	if e.CanAsk == nil || !e.CanAsk() {
		e.Note(msg.HistoryImportLater(x.Path, vm))
		return ""
	}
	if !e.Ask(msg.HistoryImportAsk(x.Path, vm)) {
		return ""
	}
	return x.Path
}

// importOffered imports the export the user said yes to (offerHistory)
// into t's new VM, started and not yet set up; none when file is "". The VM is
// made either way, so a failed import only warns, and devvm history
// import can try again.
func (e creation) importOffered(ctx context.Context, t *target, file string) error {
	if file == "" {
		return nil
	}
	err := importHistory(ctx, e.Env, t, file)
	switch m, ok := errors.AsType[msg.M](err); {
	case err == nil:
	case ctx.Err() != nil:
		return ctx.Err()
	case ok:
		e.Warn(m)
	default:
		e.Warn(msg.HistoryNotImported(file, t.f.Name))
	}
	return nil
}

// delete deletes the VM, then what devvm keeps for it (teardown): its
// folder, its proxy and link session, .git's locks or the repo's remote
// for its clone, other VMs' links to it, its host keys and its ssh config
// entry. A delete that failed but left no VM (it was half gone already)
// goes on like one that worked.
func (e Lifecycle) delete(ctx context.Context, t *target) error {
	vm := t.f.Name
	if err := t.b.Delete(ctx, vm); err != nil {
		if _, ierr := t.b.Inspect(ctx, vm); !errors.Is(ierr, backend.ErrNotFound) {
			return err
		}
	}
	e.Info(msg.Deleted(vm))
	return e.vmFolders.teardown(ctx, t, false)
}
