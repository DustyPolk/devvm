package netpolicy

import (
	"context"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// Results of the self-check, in the VM's state.
const (
	CheckOK         = "ok"
	CheckUnenforced = "unenforced"
)

// directArgv connects from the VM straight to an address on the internet,
// past its proxy settings: it must fail.
var directArgv = []string{"curl", "--noproxy", "*", "-sS", "-m", "3", "-o", "/dev/null", "https://1.1.1.1"}

// SelfCheck checks, after a VM with network policy starts, that its policy
// is enforced: on the host, that the VM runs on the process its backend
// started confined (Confined), then that a direct connection from the VM
// fails (the VM answers that one, so it only catches the honest case). The
// result goes in the VM's state.
//
// A VM devvm started confined that still gets out has another way out (a
// network devvm didn't set up), which a restart wouldn't change. One
// started some other way is offered a restart, confined (restart starts
// it again); without a terminal, or if it's still unenforced after the
// restart, devvm stops. Either way the VM is stopped first, so it isn't
// left reaching the internet. A restart declined is warned about, and the
// VM is used as it is.
func SelfCheck(ctx context.Context, b backend.Backend, st *state.VM, restart func(context.Context) error) error {
	if !On(st) {
		return nil
	}
	vm := st.Name()
	ok, err := enforced(ctx, b, vm)
	if err != nil {
		return err
	}
	if ok {
		return st.Set(KeyCheck, CheckOK)
	}
	if err := st.Set(KeyCheck, CheckUnenforced); err != nil {
		return err
	}
	confined, err := b.Confined(ctx, vm)
	if err != nil {
		return err
	}
	if confined {
		return stop(ctx, b, vm, msg.NetDirectRoute(vm))
	}
	if !terminal() {
		return stop(ctx, b, vm, msg.NetUnenforcedStop(vm))
	}
	if !confirm(msg.NetUnenforcedRestart(vm)) {
		ui.Warn(msg.NetUnenforced(vm))
		return nil
	}
	if err := b.Stop(ctx, vm); err != nil {
		return err
	}
	if err := restart(ctx); err != nil {
		return err
	}
	if ok, err := enforced(ctx, b, vm); err != nil {
		return err
	} else if !ok {
		return stop(ctx, b, vm, msg.NetUnenforcedStop(vm))
	}
	return st.Set(KeyCheck, CheckOK)
}

// Listed is a VM's network policy as devvm ls shows it: off, on, or
// unenforced: on, but the last self-check failed, or the VM runs but not
// confined (started some other way since). A stopped VM starts confined
// next time, unless that self-check failed. "" when its creation didn't
// record it.
func Listed(ctx context.Context, b backend.Backend, st *state.VM, running bool) (string, error) {
	switch {
	case st.Get(KeyNet) == "":
		return "", nil
	case !On(st):
		return "off", nil
	case st.Get(KeyCheck) == CheckUnenforced:
		return CheckUnenforced, nil
	case !running:
		return "on", nil
	}
	confined, err := b.Confined(ctx, st.Name())
	if err != nil {
		return "", err
	}
	if !confined {
		return CheckUnenforced, nil
	}
	return "on", nil
}

// enforced reports whether the VM runs confined and can't connect out
// directly.
func enforced(ctx context.Context, b backend.Backend, vm string) (bool, error) {
	confined, err := b.Confined(ctx, vm)
	if err != nil || !confined {
		return false, err
	}
	return !Direct(ctx, b, vm), nil
}

// Direct reports whether the VM reaches the internet without its proxy (it
// shouldn't). A VM that doesn't answer doesn't.
func Direct(ctx context.Context, b backend.Backend, vm string) bool {
	return b.Exec(ctx, vm, directArgv, backend.ExecOpts{}) == nil
}

// stop stops the VM, which network policy doesn't hold, and returns why:
// m, or the stop's failure after warning m.
func stop(ctx context.Context, b backend.Backend, vm string, m msg.M) error {
	if err := b.Stop(ctx, vm); err != nil {
		ui.Warn(m)
		return err
	}
	return m
}
