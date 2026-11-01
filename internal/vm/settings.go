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
// or clone for a --clone VM, which mounts no .git.
func (e Settings) GitProtect(ctx context.Context) (string, error) {
	t, err := e.folderVM(ctx, msg.NoVMGitProtect)
	if err != nil {
		return "", err
	}
	if isClone(t.st) {
		return ModeClone, nil
	}
	return e.Protector(t.f.Dir).Mode(t.st), nil
}

// SetGitProtect turns the folder's VM's .git protection on, strict or
// off. Locks change only while the VM is stopped, so nothing in it can
// race them; on and strict lock .git as it is now.
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
	if mode == gitprotect.Off && p.Mode(t.st) != gitprotect.Off {
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
