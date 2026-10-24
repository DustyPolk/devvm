package vm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// SecretSave saves a token for a known secret, given as pasted or on
// stdin. Proxies running put the new value in at once.
func (e *Env) SecretSave(ctx context.Context, name, given string) error {
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
		ui.Info(msg.SecretSavedKeychain(name))
	} else {
		ui.Info(msg.SecretSaved(store.File(name), name))
	}
	return nil
}

// SecretRm deletes a known secret's saved value. Proxies running stop
// putting it in at once.
func (e *Env) SecretRm(ctx context.Context, name string) error {
	if err := e.Proxy.Secrets.Delete(ctx, name); errors.Is(err, secrets.ErrNone) {
		return msg.NoSecret(msg.Shown(name))
	} else if err != nil {
		return err
	}
	if err := e.Proxy.RefreshSecrets(ctx); err != nil {
		return err
	}
	ui.Info(msg.SecretRemoved(name))
	return nil
}

// Secrets writes the known secrets to w as a table: whether a value is
// saved (or, for claude, in devvm's environment), and the VMs using each.
func (e *Env) Secrets(ctx context.Context, w io.Writer) error {
	names, err := state.VMNames(e.Dirs)
	if err != nil {
		return err
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
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSAVED\tUSED BY")
	for _, s := range secrets.Known {
		saved := "yes"
		if _, err := e.Proxy.Secrets.Value(ctx, s.Name); errors.Is(err, secrets.ErrNone) {
			saved = "no"
		} else if err != nil {
			return err
		}
		used := "-"
		if len(users[s.Name]) > 0 {
			used = strings.Join(users[s.Name], ", ")
		}
		_, _ = fmt.Fprintln(tw, s.Name+"\t"+saved+"\t"+used)
	}
	return tw.Flush()
}

// SecretUse binds a known secret to the folder's VM (use), or unbinds it.
// Its proxy, if it has one, puts the token in or stops at once; the VM
// gets its placeholder, or the token without network policy, from its next
// launch. Without network policy, the token it got before still works.
func (e *Env) SecretUse(ctx context.Context, name string, use bool) error {
	if _, ok := secrets.Lookup(name); !ok {
		return msg.NoSecret(msg.Shown(name))
	}
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	if !use && secrets.FromImage(st, name) {
		return msg.SecretWithImage(vm)
	}
	if err := secrets.Bind(st, name, use); err != nil {
		return err
	}
	if err := e.Proxy.RefreshSecrets(ctx, vm); err != nil {
		return err
	}
	if !use {
		ui.Info(msg.SecretUnused(name, vm))
		if !netpolicy.On(st) {
			ui.Note(msg.SecretUnuseCopied(name, vm))
		}
		return nil
	}
	ui.Info(msg.SecretUsed(name, vm))
	if _, err := e.Proxy.Secrets.Value(ctx, name); errors.Is(err, secrets.ErrNone) {
		if name == "claude" {
			ui.Warn(msg.NoClaudeToken(vm))
		} else {
			ui.Warn(msg.NoGithubToken(vm))
		}
	} else if err != nil {
		return err
	}
	return nil
}
