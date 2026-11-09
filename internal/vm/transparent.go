package vm

import (
	"context"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/netpolicy"
	"github.com/devvmhq/devvm/internal/state"
)

// Transparent returns the folder's VM's transparent mode and where it
// comes from (netpolicy.TransparentOf): whether, while it runs confined,
// the names that tools without proxy settings look up lead to its proxy.
func (e Settings) Transparent(ctx context.Context) (netpolicy.Transparent, error) {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return netpolicy.Transparent{}, err
	}
	return netpolicy.TransparentOf(e.Dirs, t.st)
}

// SetTransparent gives the folder's VM its own transparent mode, v
// (netpolicy.TransparentOn or TransparentOff), as command (devvm network
// transparent off) asks. It applies at the VM's next launch, whose setup
// turns it on or off in the VM (sync), and only while the VM runs
// confined: no restart.
func (e Settings) SetTransparent(ctx context.Context, v, command string) error {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm := t.f.Name
	if n, err := netpolicy.Of(t.st); err == nil && n.Transparent == v {
		e.Info(msg.SettingAlready(command, vm))
		return nil
	}
	if err := t.st.Update(ctx, func(st *state.VM) error { return netpolicy.SetTransparent(st, v) }); err != nil {
		return err
	}
	e.Info(msg.NetTransparentSet(v, vm))
	return nil
}
