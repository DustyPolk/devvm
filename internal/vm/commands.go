package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/ui"
)

// existing finds the folder's VM, which must exist, be made for this
// folder and have finished being made.
func (e *Env) existing(ctx context.Context, flag string) (*found, error) {
	t, err := e.findExisting(ctx, flag)
	if err != nil {
		return nil, err
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Exists() || t.st.Get(KeyNet) == "" {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// Restart stops the folder's VM, then opens it as Up does: what changes
// only when it starts (its mounts, size, ports) applies.
func (e *Env) Restart(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	if err := t.b.Stop(ctx, t.f.Name); err != nil {
		return err
	}
	o.Image = ""
	return e.open(ctx, o, t)
}

// Remove takes shared tools (names joined with +) out of the folder's VM,
// then opens it: they leave its image name and its list, and aren't
// mounted from its next launch (an agent's folders neither). What the VM
// installed itself stays.
func (e *Env) Remove(ctx context.Context, o Options, names string) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	img, files, err := tools.Remove(vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), image.Normalize(names))
	if err != nil {
		return err
	}
	if err := st.Set(state.KeyImage, img); err != nil {
		return err
	}
	if err := st.Set(KeyTools, strings.Join(files, " ")); err != nil {
		return err
	}
	ui.Info(msg.VMIsNow(img, vm))
	o.Image = ""
	return e.open(ctx, o, t)
}

// Update checks for newer shared tools and moves the folder's VM to them,
// built first, then opens it. Nothing in its disk changes: its tools are
// mounted from the share.
func (e *Env) Update(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	builder := e.builder(t.b, st.Get(state.KeyFrom))
	libc := st.Get(KeyLibc)
	buildFn := func(ctx context.Context, files []string) error { return e.buildTools(ctx, builder, libc, files) }
	files, changed, err := e.Tools.Update(ctx, buildFn, vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), libc)
	if err != nil {
		return err
	}
	if changed {
		if err := st.Set(KeyTools, strings.Join(files, " ")); err != nil {
			return err
		}
	}
	o.Image = ""
	return e.open(ctx, o, t)
}

// Adopt gives the folder the VM made for an earlier folder at its path,
// after asking: its files, tokens and allowlist. Its .git is locked as
// new at its next launch.
func (e *Env) Adopt(ctx context.Context) error {
	t, err := e.findExisting(ctx, "")
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	if e.sameFolder(t.f, st) {
		ui.Info(msg.VMAlreadyHere(vm))
		return nil
	}
	was := st.Get(state.KeyDir)
	if !terminal() {
		return msg.VMOtherFolderAdopt(vm, was)
	}
	if !ask(msg.AdoptVM(vm, was)) {
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
	ui.Info(msg.VMAdopted(t.f.Dir, vm))
	return nil
}

// Rm deletes the folder's VM, exporting its agents' history first unless
// force: an export that fails keeps it. A --clone VM's branches the repo
// lacks and uncommitted changes are checked first (cloneRmCheck), unless
// force. A VM made for an earlier folder at this path is deleted only
// after asking, without either (its folder is gone), force or not.
func (e *Env) Rm(ctx context.Context, force bool) error {
	t, err := e.findExisting(ctx, "")
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	if !e.sameFolder(t.f, st) {
		was := st.Get(state.KeyDir)
		if !terminal() {
			return msg.VMOtherFolderRm(vm, was)
		}
		if !ask(msg.DeleteOtherFolderVM(vm, was)) {
			return nil
		}
		force = true
	}
	// A VM whose creation didn't finish never ran: no history, no work.
	if !force && st.Get(KeyNet) != "" && isClone(st) {
		keep, err := e.cloneRmCheck(ctx, t)
		if err != nil || keep {
			return err
		}
	}
	if !force && st.Get(KeyNet) != "" && len(agentsOf(st.Get(state.KeyImage))) > 0 {
		if err := e.export(ctx, t); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// ensureRunning starts the VM if it's stopped, confined as its state says.
func (e *Env) ensureRunning(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	in, err := b.Inspect(ctx, vm)
	if err != nil || in.Status == backend.StatusRunning {
		return err
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if netpolicy.On(t.st) {
		conf.Net = backend.NetProxy
	}
	if err := backend.Require(b, vm, conf.Features()...); err != nil {
		return err
	}
	ui.Info(msg.Starting(vm))
	_, err = e.start(ctx, b, t.st, conf)
	return err
}

// export saves the VM's agents' history (transcripts, sessions) to a
// tarball in the exports folder, starting the VM if it's stopped. Their
// logins aren't in it.
func (e *Env) export(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	if err := e.ensureRunning(ctx, t); err != nil {
		return err
	}
	if err := os.MkdirAll(e.Dirs.Exports(), 0o700); err != nil {
		return err
	}
	file := filepath.Join(e.Dirs.Exports(), vm+"-"+time.Now().Format("20060102-150405")+".tar.gz")
	out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // devvm's own folder
	if err != nil {
		return err
	}
	excludes := make([]string, len(historyExcludes))
	for i, p := range historyExcludes {
		excludes[i] = "--exclude=" + p
	}
	script := "cd ~ && ls -d " + strings.Join(historyPaths, " ") + " 2>/dev/null | tar -czf - " + strings.Join(excludes, " ") + " -T -"
	err = b.Exec(ctx, vm, []string{"bash", "-c", script}, backend.ExecOpts{Stdout: out, Stderr: ui.LogWriter()})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(file)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return msg.ExportFailed(vm)
	}
	ui.Info(msg.Exported(file, vm))
	return nil
}

// delete deletes the VM and what devvm keeps for it: its state, its proxy
// and its rules, run files and logs, .git's locks or the repo's remote
// for its clone, its host keys and its ssh config entry. A delete that
// failed but left no VM (it was half gone already) goes on like one that
// worked.
func (e *Env) delete(ctx context.Context, t *found) error {
	vm := t.f.Name
	if err := t.b.Delete(ctx, vm); err != nil {
		if _, ierr := t.b.Inspect(ctx, vm); !errors.Is(ierr, backend.ErrNotFound) {
			return err
		}
	}
	ui.Info(msg.Deleted(vm))
	if isClone(t.st) {
		removeRemote(ctx, t.f.Dir, vm, t.st.Get(KeyCloneRemote))
	} else if t.st.Get(gitprotect.KeyGitProtect) == "on" {
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

// LogOptions are what devvm log shows of the network log.
type LogOptions struct {
	Lines   int  // the last events shown
	Follow  bool // then each new one, until ctx ends
	Blocked bool // the blocks grouped by host instead
}

// Log writes the folder's VM's network log to w: what its proxy allowed
// and blocked. A VM without network policy logs nothing new, and devvm
// says so.
func (e *Env) Log(ctx context.Context, w io.Writer, o LogOptions) error {
	t, err := e.findExisting(ctx, "")
	if err != nil {
		return err
	}
	vm := t.f.Name
	if !netpolicy.On(t.st) {
		ui.Note(msg.NetOffLog(vm))
	}
	path := netpolicy.EventLog(e.Dirs, vm)
	// Its proxy makes the log as it starts: following an empty one waits.
	if fi, err := os.Stat(path); errors.Is(err, os.ErrNotExist) || err == nil && fi.Size() == 0 && !o.Follow {
		ui.Info(msg.LogEmpty(vm))
		return nil
	}
	if o.Blocked {
		return netpolicy.WriteBlocked(w, path)
	}
	return netpolicy.WriteLog(ctx, w, path, o.Lines, o.Follow)
}
