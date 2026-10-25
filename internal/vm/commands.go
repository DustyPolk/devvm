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
	"github.com/w3cj/devvm/internal/sshagent"
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

// Stop stops the folder's VM and its proxy. Then, the VM being stopped,
// .git is locked again and checked (on Linux, set-id files too) as on a
// launch that finds it stopped, and its ssh config entry goes.
func (e *Env) Stop(ctx context.Context) error {
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
	e.Info(msg.VMIsNow(img, vm))
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
	tg := toolTarget(t.b, st)
	buildFn := func(ctx context.Context, files []string) error { return e.buildTools(ctx, builder, tg, files) }
	files, changed, err := e.Tools.Update(ctx, buildFn, vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), tg)
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
		e.Info(msg.VMAlreadyHere(vm))
		return nil
	}
	was := st.Get(state.KeyDir)
	if !terminal() {
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
		if !e.Ask(msg.DeleteOtherFolderVM(vm, was)) {
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
		if err := e.export(ctx, t, e.exportFile(vm), msg.RmExportFailed(vm)); err != nil {
			return err
		}
	}
	return e.delete(ctx, t)
}

// Export saves the folder's VM's agents' history to file (absolute), or
// to the exports folder when it's empty, starting the VM if it's stopped.
func (e *Env) Export(ctx context.Context, file string) error {
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	vm := t.f.Name
	if file == "" {
		file = e.exportFile(vm)
	}
	return e.export(ctx, t, file, backend.Logged(msg.HistoryNotExported(vm)))
}

// exportFile is a new export's file in the exports folder.
func (e *Env) exportFile(vm string) string {
	return filepath.Join(e.Dirs.Exports(), vm+"-"+time.Now().Format("20060102-150405")+".tar.gz")
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
	e.Info(msg.Starting(vm))
	_, err = e.start(ctx, b, t.st, conf)
	return err
}

// export saves the VM's agents' history (transcripts, sessions) to a
// tarball at file, starting the VM if it's stopped; failed is the error
// when its tar fails. Their logins aren't in it. It's written next to
// file and renamed into place once complete, so a failed export leaves
// nothing and replaces nothing.
func (e *Env) export(ctx context.Context, t *found, file string, failed error) error {
	vm, b := t.f.Name, t.b
	if err := e.ensureRunning(ctx, t); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(file), ".devvm-export-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	done := ui.AddCleanup(func(context.Context) { _ = os.Remove(tmp) })
	defer func() {
		_ = os.Remove(tmp)
		done()
	}()
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
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failed
	}
	if err := os.Rename(tmp, file); err != nil {
		return err
	}
	e.Info(msg.Exported(file, vm))
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
	e.Info(msg.Deleted(vm))
	if isClone(t.st) {
		e.removeRemote(ctx, t.f.Dir, vm, t.st.Get(KeyCloneRemote))
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

// Network writes the folder's VM's network policy to w, as devvm ls
// shows it: on, off or unenforced.
func (e *Env) Network(ctx context.Context, w io.Writer) error {
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	in, err := t.b.Inspect(ctx, t.f.Name)
	if err != nil {
		return err
	}
	shown, err := netpolicy.Listed(ctx, t.b, t.st, in.Status == backend.StatusRunning)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, shown+"\n")
	return err
}

// SetNetwork turns the folder's VM's network policy on or off, from its
// next start. Turning it on, its backend must be able to confine the VM
// to its proxy, and tokens the VM got before stay valid.
func (e *Env) SetNetwork(ctx context.Context, on bool) error {
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	to := "off"
	if on {
		to = "on"
	}
	if st.Get(KeyNet) == to {
		e.Info(msg.NetAlready(to, vm))
		return nil
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	running := in.Status == backend.StatusRunning
	if on {
		if err := backend.Require(b, vm, backend.Confinement{Net: backend.NetProxy}.Features()...); err != nil {
			return err
		}
	}
	if err := st.Set(KeyNet, to); err != nil {
		return err
	}
	if running {
		e.Info(msg.NetSetRunning(to, vm))
	} else {
		e.Info(msg.NetSetStopped(to, vm))
	}
	if on {
		e.Note(msg.NetOnOldTokens(vm))
	}
	return nil
}

// GitProtect writes the folder's VM's .git protection to w: on, strict,
// off, or clone for a --clone VM, which mounts no .git.
func (e *Env) GitProtect(ctx context.Context, w io.Writer) error {
	t, err := e.folderVM(ctx, msg.NoVMGitProtect)
	if err != nil {
		return err
	}
	mode := ModeClone
	if !isClone(t.st) {
		mode = e.Protector(t.f.Dir).Mode(t.st)
	}
	_, err = io.WriteString(w, mode+"\n")
	return err
}

// SetGitProtect turns the folder's VM's .git protection on, strict or
// off. Locks change only while the VM is stopped, so nothing in it can
// race them; on and strict lock .git as it is now.
func (e *Env) SetGitProtect(ctx context.Context, mode string) error {
	t, err := e.folderVM(ctx, msg.NoVMGitProtect)
	if err != nil {
		return err
	}
	vm := t.f.Name
	if isClone(t.st) {
		return msg.GitProtectClone(vm)
	}
	p := e.Protector(t.f.Dir)
	// Elsewhere there are no locks to race.
	if p.OS == "darwin" {
		in, err := t.b.Inspect(ctx, vm)
		if err != nil {
			return err
		}
		if in.Status == backend.StatusRunning {
			return msg.GitProtectRunning(vm)
		}
	}
	if err := p.Set(ctx, t.st, mode); err != nil {
		return err
	}
	switch _, err := os.Lstat(filepath.Join(t.f.Dir, ".git")); {
	case mode == gitprotect.Off:
		e.Info(msg.GitUnlocked(vm))
	case err != nil:
		e.Info(msg.GitLockLater(vm))
	case mode == gitprotect.Strict:
		e.Info(msg.GitLockedStrict(vm))
	default:
		e.Info(msg.GitLocked(vm))
	}
	return nil
}

// agentVM finds the folder's VM for devvm ssh-agent.
func (e *Env) agentVM(ctx context.Context) (*found, error) { return e.folderVM(ctx, msg.NoVMSSHAgent) }

// folderVM finds the folder's VM for a command that shows or changes it,
// which must exist (else noVM), be made for this folder and have finished
// being made.
func (e *Env) folderVM(ctx context.Context, noVM func(dir string) msg.M) (*found, error) {
	t, err := e.find(ctx, "")
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, noVM(t.f.Dir)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Exists() || t.st.Get(KeyNet) == "" {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// SSHAgent writes the folder's VM's SSH agent forwarding to w: off, on
// (the user's agent) or devvm (devvm's own).
func (e *Env) SSHAgent(ctx context.Context, w io.Writer) error {
	t, err := e.agentVM(ctx)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, sshagent.Mode(t.st)+"\n")
	return err
}

// SetSSHAgent changes the folder's VM's SSH agent forwarding, from its
// next start: off, on (the user's agent) or devvm (devvm's own, started
// with its keys loaded now).
func (e *Env) SetSSHAgent(ctx context.Context, mode string) error {
	t, err := e.agentVM(ctx)
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	if sshagent.Mode(st) == mode {
		e.Info(msg.AgentAlready(mode, vm))
		return nil
	}
	if sshagent.Forwards(mode) {
		if err := backend.Require(b, vm, backend.FeatureAgentForwarding); err != nil {
			return err
		}
	}
	if mode == sshagent.Devvm {
		if err := e.Agent.Ensure(ctx); err != nil {
			return err
		}
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if sshagent.Forwards(mode) != in.AgentForwarding {
		if err := backend.Require(b, vm, backend.FeatureAgentToggle); err != nil {
			return err
		}
	}
	if err := st.Set(sshagent.Key, mode); err != nil {
		return err
	}
	switch mode {
	case sshagent.On:
		e.Warn(msg.AgentOnRisk(vm))
	case sshagent.Devvm:
		e.Warn(msg.AgentDevvmRisk(vm))
	}
	if in.Status == backend.StatusRunning {
		e.Info(msg.AgentSetRunning(mode, vm))
	} else {
		e.Info(msg.AgentSetStopped(mode, vm))
	}
	return nil
}

// AgentAdd loads a key into devvm's own SSH agent, and records it.
func (e *Env) AgentAdd(ctx context.Context, file string, o sshagent.AddOptions) error {
	return e.Agent.Add(ctx, file, o)
}

// AgentKeys writes to w the keys devvm's own SSH agent holds, then those
// added to it.
func (e *Env) AgentKeys(ctx context.Context, w io.Writer) error {
	return e.Agent.Keys(ctx, w)
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
		e.Note(msg.NetOffLog(vm))
	}
	path := netpolicy.EventLog(e.Dirs, vm)
	// Its proxy makes the log as it starts: following an empty one waits.
	if fi, err := os.Stat(path); errors.Is(err, os.ErrNotExist) || err == nil && fi.Size() == 0 && !o.Follow {
		e.Info(msg.LogEmpty(vm))
		return nil
	}
	if o.Blocked {
		return netpolicy.WriteBlocked(w, path)
	}
	return netpolicy.WriteLog(ctx, w, path, o.Lines, o.Follow)
}

// policyVM finds the folder's VM for devvm policy, which must exist and
// be made for this folder; noVM is the error when it doesn't exist.
func (e *Env) policyVM(ctx context.Context, noVM func(dir string) msg.M) (*found, error) {
	t, err := e.find(ctx, "")
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, noVM(t.f.Dir)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !netpolicy.On(t.st) {
		e.Note(msg.NetOffPolicy(t.f.Name))
	}
	return t, nil
}

// Policy writes the folder's VM's allowlist to w: each policy file its
// proxy reads that has rules, in order.
func (e *Env) Policy(ctx context.Context, w io.Writer) error {
	t, err := e.policyVM(ctx, msg.NoVM)
	if err != nil {
		return err
	}
	return netpolicy.WriteRules(w, netpolicy.PolicyFiles(e.Dirs, e.Root, t.f.Name, t.st.Get(state.KeyImage)))
}

// PolicyChange adds rules to the folder's VM's own policy file (deny: as
// denies), or with remove takes them out; with global, the user's file
// for every VM instead. Its proxy reads the file again at its next
// connection, so a change applies at once.
func (e *Env) PolicyChange(ctx context.Context, rules []string, deny, remove, global bool) error {
	file, vm := e.Dirs.Policy(), ""
	if !global {
		t, err := e.policyVM(ctx, msg.NoVMPolicy)
		if err != nil {
			return err
		}
		vm = t.f.Name
		file = netpolicy.VMPolicyFile(e.Dirs, vm)
	}
	if remove {
		for _, r := range rules {
			switch ok, err := netpolicy.RemoveRule(file, r); {
			case err != nil:
				return err
			case !ok:
				e.Warn(msg.RuleMissing(file, msg.Shown(r)))
			case global:
				e.Info(msg.RuleRemovedGlobal(r))
			default:
				e.Info(msg.RuleRemoved(r, vm))
			}
		}
		return nil
	}
	// Every rule is checked before any is added.
	checked := make([]string, len(rules))
	for i, r := range rules {
		var err error
		if checked[i], err = netpolicy.Rule(r, deny); err != nil {
			return err
		}
	}
	for _, r := range checked {
		switch ok, err := netpolicy.AddRule(file, r); {
		case err != nil:
			return err
		case !ok:
			e.Info(msg.RulePresent(file, r))
		case global:
			e.Info(msg.RuleAddedGlobal(r))
		default:
			e.Info(msg.RuleAdded(r, vm))
		}
	}
	return nil
}
