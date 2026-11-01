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

// Stop stops the folder's VM and its proxy. Then, the VM being stopped,
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
	if !isClone(t.st) {
		if err := e.Protector(t.f.Dir).Check(ctx, t.st, false); err != nil {
			return err
		}
		e.checkSetID(t.f.Dir)
	}
	return e.sshConfig().Write(ctx)
}

// Adopt gives the folder the VM made for an earlier folder at its path,
// after asking: its files, tokens and allowlist. Its .git is locked as
// new at its next launch. Only the VM named for this path is adopted,
// never a parent folder's.
func (e Lifecycle) Adopt(ctx context.Context) error {
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
	if !e.Terminal() {
		return msg.VMOtherFolderAdopt(vm, was)
	}
	if !e.Ask(msg.AdoptVM(vm, was)) {
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

// Rm deletes the folder's VM, exporting its agents' history first unless
// force: an export that fails keeps it. A --clone VM's branches the repo
// lacks and uncommitted changes are checked first (cloneRmCheck), unless
// force. A VM made for an earlier folder at this path is deleted only
// after asking, without either (its folder is gone), force or not. A
// parent folder's VM, found from a subfolder, is deleted only after
// asking too, and never without a terminal.
func (e Lifecycle) Rm(ctx context.Context, force bool) error {
	t, err := e.findExisting(ctx, "")
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	// A parent folder's VM is deleted only after asking, -f or not; -f
	// still skips the checks below.
	if t.sub != "" {
		if !e.Terminal() {
			return msg.RmParentVM(t.f.Dir, vm)
		}
		if !e.Ask(msg.DeleteParentVM(t.f.Dir, vm)) {
			return nil
		}
	}
	if !e.sameFolder(t.f, st) {
		was := st.Get(state.KeyDir)
		if !e.Terminal() {
			return msg.VMOtherFolderRm(vm, was)
		}
		if !e.Ask(msg.DeleteOtherFolderVM(vm, was)) {
			return nil
		}
		force = true
	}
	// A VM whose creation didn't finish never ran: no history, no work.
	if !force && st.Get(KeyNet) != "" && isClone(st) {
		keep, err := clones(e).cloneRmCheck(ctx, t)
		if err != nil || keep {
			return err
		}
	}
	if !force && st.Get(KeyNet) != "" && len(agentsOf(st.Get(state.KeyImage))) > 0 {
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
// clone's remote or .git's locks, its proxy and its rules, run files,
// state and logs, its host keys and its ssh config entry.
func (e Lifecycle) forget(ctx context.Context, t *found) error {
	vm := t.f.Name
	if isClone(t.st) {
		clones(e).removeRemote(ctx, t.f.Dir, vm, t.st.Get(KeyCloneRemote))
	} else if m := t.st.Get(gitprotect.KeyGitProtect); m == gitprotect.On || m == gitprotect.Strict {
		e.Protector(t.f.Dir).Unlock(ctx)
	}
	if err := e.Proxy.Forget(vm); err != nil {
		return err
	}
	if err := t.st.Remove(); err != nil {
		return err
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
