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
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Lifecycle stops the folder's VM, gives it to the folder, exports its
// agents' history and deletes it.
type Lifecycle struct{ *Env }

// Stop stops the folder's VM, its proxy and its link session. Then, the
// VM being stopped,
// .git is locked again and checked (on Linux, set-id files too) as on a
// launch that finds it stopped, and its ssh config entry goes.
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
	if !isClone(t.st) {
		if err := e.Protector(t.f.Dir).Check(ctx, t.st, false); err != nil {
			return err
		}
		e.checkSetID(t.f.Dir)
	}
	return e.sshConfig().Write(ctx)
}

// Adopt gives the folder the VM made for an earlier folder at its path,
// after asking in a terminal, or with force (--force): its files, tokens
// and allowlist. Its .git is locked as new at its next launch. Only the
// VM named for this path is adopted, never a parent folder's.
func (e Lifecycle) Adopt(ctx context.Context, force bool) error {
	t, _, err := e.lookup(ctx, "", false)
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
	was := st.Get(state.KeyDir)
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
	for _, kv := range [][2]string{{state.KeyDir, t.f.Dir}, {state.KeyFolder, id}, {gitprotect.KeyGitInode, ""}} {
		if err := st.Set(kv[0], kv[1]); err != nil {
			return err
		}
	}
	e.Info(msg.VMAdopted(t.f.Dir, vm))
	return nil
}

// RmOptions are devvm rm's flags.
type RmOptions struct {
	Force    bool // --force: no "delete it?" asked, which needs a terminal otherwise
	NoExport bool // --no-export: the agents' history isn't exported first
}

// Rm deletes the folder's VM, after asking in a terminal, or with
// --force; without either it refuses (force_needed). Its agents' history
// is exported first unless --no-export: an export that fails keeps it.
// A --clone VM's branches the repo lacks and uncommitted changes are
// checked first (cloneRmCheck), which asks only in a terminal. A VM made
// for an earlier folder at this path, or a parent folder's VM found from
// a subfolder, is deleted only after asking in a terminal, which --force
// never answers, and never without one; the earlier folder's isn't
// exported, as its folder is gone.
func (e Lifecycle) Rm(ctx context.Context, o RmOptions) error {
	t, err := e.findExisting(ctx, "")
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
		was := st.Get(state.KeyDir)
		if !e.Terminal() {
			return msg.VMOtherFolderRm(vm, was)
		}
		if !e.Ask(msg.DeleteOtherFolderVM(vm, was)) {
			return nil
		}
		asked, o.NoExport = true, true
	}
	if !asked && !o.Force {
		if !e.Terminal() {
			return msg.ForceNeeded("devvm rm")
		}
		if !e.Ask(msg.RmConfirm(vm)) {
			e.Info(msg.Kept(vm))
			return nil
		}
	}
	// A VM whose creation didn't finish never ran: no history, no work.
	if st.Get(KeyNet) != "" && isClone(st) && e.sameFolder(t.f, st) {
		keep, err := clones(e).cloneRmCheck(ctx, t)
		if err != nil || keep {
			return err
		}
	}
	if !o.NoExport && st.Get(KeyNet) != "" && len(agentsOf(st.Get(state.KeyImage))) > 0 {
		if err := e.export(ctx, t, "", msg.RmExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// Export saves the folder's VM's agents' history to file (absolute), or
// to the exports folder when it's empty, starting the VM if it's stopped.
func (e Lifecycle) Export(ctx context.Context, file string) error {
	t, err := e.existing(ctx, "")
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
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	if filepath.Dir(file) == e.Dirs.Exports() {
		if err := os.Chmod(e.Dirs.Exports(), 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return err
		}
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

// delete deletes the VM and what devvm keeps for it: its state, its proxy
// and its rules, run files and logs, .git's locks or the repo's remote
// for its clone, its host keys and its ssh config entry. A delete that
// failed but left no VM (it was half gone already) goes on like one that
// worked.
func (e Lifecycle) delete(ctx context.Context, t *found) error {
	vm := t.f.Name
	if err := t.b.Delete(ctx, vm); err != nil {
		if _, ierr := t.b.Inspect(ctx, vm); !errors.Is(ierr, backend.ErrNotFound) {
			return err
		}
	}
	e.Info(msg.Deleted(vm))
	return e.forget(ctx, t)
}

// forget deletes what devvm keeps for a VM its backend no longer has: its
// clone's remote or .git's locks, its proxy and its rules, its link
// session, run files, state (its own links with it) and logs, its host
// keys and its ssh config entry. Other VMs' links to it go too
// (unlinkTarget), and it leaves its group.
func (e Lifecycle) forget(ctx context.Context, t *found) error {
	vm := t.f.Name
	if isClone(t.st) {
		clones(e).removeRemote(ctx, t.f.Dir, vm, t.st.Get(KeyCloneRemote))
	} else if m := t.st.Get(gitprotect.KeyGitProtect); m == gitprotect.On || m == gitprotect.Strict {
		e.Protector(t.f.Dir).Unlock(ctx)
	}
	linkRuns(e).stop(ctx, vm)
	if err := e.Proxy.Forget(vm); err != nil {
		return err
	}
	group := t.st.Get(KeyGroup)
	if err := t.st.Remove(); err != nil {
		return err
	}
	if err := linkRuns(e).unlinkTarget(ctx, vm); err != nil {
		return err
	}
	if group != "" {
		if err := Groups(e).leave(ctx, t.st, group); err != nil {
			return err
		}
	}
	logs, err := filepath.Glob(filepath.Join(e.Dirs.Logs(), vm+".*"))
	if err != nil {
		return err
	}
	for _, f := range logs {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := e.sshConfig().ForgetHostKeys(vm); err != nil {
		return err
	}
	return e.sshConfig().Write(ctx)
}
