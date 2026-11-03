package vm

import (
	"context"
	"os"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
)

// Settings shows and changes the folder's VM's settings: its .git
// protection (here), its size (size.go), its ports (ports.go) and its SSH
// agent forwarding (sshagent.go).
type Settings struct{ *Env }

// GitProtect returns the folder's VM's .git protection: on, strict, off,
// or clone for a --clone VM, which mounts no .git (gitprotect.Mode, as
// devvm ls shows it).
func (e Settings) GitProtect(ctx context.Context) (string, error) {
	t, err := e.folderVM(ctx, msg.NoVMGitProtect)
	if err != nil {
		return "", err
	}
	return gitprotect.Mode(t.st), nil
}

// SetGitProtect turns the folder's VM's .git protection on, strict or
// off. Locks change only while the VM is stopped, so nothing in it can
// race them: a running VM's choice is saved, and set when devvm next
// finds it stopped (checkGit), which a restart the user agrees to does
// now (restartOr). On and strict lock .git as it is then.
func (e Settings) SetGitProtect(ctx context.Context, mode string) error {
	t, err := e.folderVM(ctx, msg.NoVMGitProtect)
	if err != nil {
		return err
	}
	vm := t.f.Name
	if isClone(t.st) {
		return msg.GitProtectClone(vm)
	}
	p := e.Protector(t.f.Dir)
	if mode == gitprotect.Off && gitprotect.Mode(t.st) != gitprotect.Off {
		if ok, err := t.widen(e.Env, "devvm git-protect off"); !ok {
			return err
		}
	}
	// Elsewhere there are no locks to race.
	if p.OS == "darwin" {
		in, err := t.b.Inspect(ctx, vm)
		if err != nil {
			return err
		}
		if in.Status == backend.StatusRunning {
			if err := gitprotect.SetNext(ctx, t.st, mode); err != nil {
				return err
			}
			return e.restartOr(ctx, vm, e.Info, msg.GitProtectNextStop(mode, vm))
		}
	}
	return setGitProtect(ctx, e.Env, p, t, mode)
}

// setGitProtect sets a stopped VM's .git protection, and says what it
// did.
func setGitProtect(ctx context.Context, e *Env, p *gitprotect.Protector, t *found, mode string) error {
	vm := t.f.Name
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

// checkGit is what devvm stop and each launch of an existing VM do for
// its project folder, unless it's a --clone VM, which mounts none of it:
// .git is checked (gitprotect's Check) under the protection the VM ran
// with, so what it changed meanwhile is named; then, once the VM is
// stopped, the protection chosen while it ran is set, and on Linux
// set-id files are checked.
func checkGit(ctx context.Context, e *Env, t *found, running bool) error {
	if isClone(t.st) {
		return nil
	}
	p := e.Protector(t.f.Dir)
	if err := p.Check(ctx, t.st, running); err != nil {
		return err
	}
	if next := gitprotect.Next(t.st); next != "" && !running {
		if err := setGitProtect(ctx, e, p, t, next); err != nil {
			return err
		}
	}
	e.checkSetID(t.f.Dir)
	return nil
}
