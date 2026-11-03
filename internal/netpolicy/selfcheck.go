package netpolicy

import (
	"context"
	"errors"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
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
// started some other way, or before its network policy was turned on, is
// offered a restart, confined (restart starts
// it again); without a terminal, or if it's still unenforced after the
// restart, devvm stops. Either way the VM is stopped first, so it isn't
// left reaching the internet. A restart declined is warned about, and the
// VM is used as it is.
func (p *Proxy) SelfCheck(ctx context.Context, b backend.Backend, st *state.VM, restart func(context.Context) error) error {
	if !On(st) {
		return nil
	}
	vm := st.Name()
	ok, err := enforced(ctx, b, vm)
	if err != nil {
		return err
	}
	if ok {
		return setCheck(ctx, st, CheckOK)
	}
	if err := setCheck(ctx, st, CheckUnenforced); err != nil {
		return err
	}
	confined, err := b.Confined(ctx, vm)
	if err != nil {
		return err
	}
	if confined {
		return p.stop(ctx, b, vm, msg.NetDirectRoute(vm))
	}
	if !p.Terminal() {
		return p.stop(ctx, b, vm, msg.NetUnenforcedStop(vm))
	}
	if !p.Confirm(msg.NetUnenforcedRestart(vm)) {
		p.Warn(msg.NetUnenforced(vm))
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
		return p.stop(ctx, b, vm, msg.NetUnenforcedStop(vm))
	}
	return setCheck(ctx, st, CheckOK)
}

// setCheck records the self-check's result in the VM's state.
func setCheck(ctx context.Context, st *state.VM, result string) error {
	return st.Update(ctx, func(st *state.VM) error {
		n, err := Of(st)
		if err != nil {
			return err
		}
		n.Made.Check = result
		return Set(st, n)
	})
}

// Listed is a VM's network policy as devvm ls shows it: off, on, or
// unenforced: on, but the last self-check failed, or the VM runs but not
// confined (started some other way since). A stopped VM starts confined
// next time, unless that self-check failed. "" when its creation didn't
// finish.
func Listed(ctx context.Context, b backend.Backend, st *state.VM, running bool) (string, error) {
	n, err := Of(st)
	switch {
	case err != nil:
		return "", err
	case !st.Finished():
		return "", nil
	case !n.On:
		return "off", nil
	case n.Made.Check == CheckUnenforced:
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

// Unproxied stops a running VM whose proxy couldn't start (err): a VM with
// network policy never runs on without its proxy. It warns that it stopped
// it and returns err; if the stop fails, the stop's failure, after warning
// err. The stop isn't cancelled with ctx: an interrupt never leaves the VM
// running.
func (p *Proxy) Unproxied(ctx context.Context, b backend.Backend, vm string, err error) error {
	if serr := b.Stop(context.WithoutCancel(ctx), vm); serr != nil {
		var m msg.M
		if errors.As(err, &m) {
			p.Warn(m)
		}
		return serr
	}
	p.Warn(msg.NetProxyDownStopped(vm))
	return err
}

// stop stops the VM, which network policy doesn't hold, and returns why:
// m, or the stop's failure after warning m.
func (p *Proxy) stop(ctx context.Context, b backend.Backend, vm string, m msg.M) error {
	if err := b.Stop(ctx, vm); err != nil {
		p.Warn(m)
		return err
	}
	return m
}
