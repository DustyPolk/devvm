package vm

import (
	"context"
	"errors"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
)

// SecretSave saves a token for a known secret, given as pasted or on
// stdin. Proxies running put the new value in at once.
func (e Net) SecretSave(ctx context.Context, name, given string) error {
	v, err := secrets.Token(name, given)
	if err != nil {
		return err
	}
	store := e.Proxy.Secrets
	if err := store.Save(ctx, name, v); err != nil {
		return err
	}
	if err := e.Proxy.RefreshSecrets(ctx); err != nil {
		return err
	}
	if store.Keychain {
		e.Info(msg.SecretSavedKeychain(name))
	} else {
		e.Info(msg.SecretSaved(store.File(name), name))
	}
	return nil
}

// SecretRm deletes a known secret's saved value. Proxies running stop
// putting it in at once.
func (e Net) SecretRm(ctx context.Context, name string) error {
	if err := e.Proxy.Secrets.Delete(ctx, name); errors.Is(err, secrets.ErrNone) {
		return msg.NoSecret(name)
	} else if err != nil {
		return err
	}
	if err := e.Proxy.RefreshSecrets(ctx); err != nil {
		return err
	}
	e.Info(msg.SecretRemoved(name))
	return nil
}

// SecretListed is a known secret as devvm secret lists it: never its
// value.
type SecretListed struct {
	Name string
	// Saved is whether a value is saved (or, for claude, in devvm's
	// environment).
	Saved  bool
	UsedBy []string // the VMs using it, by name
}

// Secrets returns the known secrets, each with whether a value is saved
// and the VMs using it.
func (e Net) Secrets(ctx context.Context) ([]SecretListed, error) {
	names, err := state.VMNames(e.Dirs)
	if err != nil {
		return nil, err
	}
	users := map[string][]string{}
	for _, vm := range names {
		st, err := state.ReadVM(e.Dirs, vm)
		if err != nil {
			continue // not a VM's state
		}
		for _, s := range secrets.Names(st) {
			users[s] = append(users[s], vm)
		}
	}
	out := make([]SecretListed, 0, len(secrets.Known))
	for _, s := range secrets.Known {
		saved := true
		if _, err := e.Proxy.Secrets.Value(ctx, s.Name); errors.Is(err, secrets.ErrNone) {
			saved = false
		} else if err != nil {
			return nil, err
		}
		out = append(out, SecretListed{Name: s.Name, Saved: saved, UsedBy: users[s.Name]})
	}
	return out, nil
}

// SecretUse binds a known secret to the folder's VM (use), or unbinds it.
// Its proxy, if it has one, puts the token in or stops at once; the VM
// gets its placeholder, or the token without network policy, from its next
// launch. Without network policy, the token it got before still works.
func (e Net) SecretUse(ctx context.Context, name string, use bool) error {
	if _, ok := secrets.Lookup(name); !ok {
		return msg.NoSecret(name)
	}
	t, err := e.existing(ctx)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	if !use && secrets.FromTools(st, name) {
		return msg.SecretWithClaude(vm)
	}
	if use {
		if ok, err := t.widen(e.Env, "devvm secret use"); !ok {
			return err
		}
	}
	if err := secrets.Bind(st, name, use); err != nil {
		return err
	}
	if err := e.Proxy.RefreshSecrets(ctx, vm); err != nil {
		return err
	}
	if !use {
		e.Info(msg.SecretUnused(name, vm))
		if !netpolicy.On(st) {
			e.Note(msg.SecretUnuseCopied(name, vm))
		}
		return nil
	}
	e.Info(msg.SecretUsed(name, vm))
	if _, err := e.Proxy.Secrets.Value(ctx, name); errors.Is(err, secrets.ErrNone) {
		if name == "claude" {
			e.Warn(msg.NoClaudeToken(vm))
		} else {
			e.Warn(msg.NoGithubToken(vm))
		}
	} else if err != nil {
		return err
	}
	return nil
}
