package vm

import (
	"context"
	"net"
	"net/netip"
	"strconv"

	"github.com/devvmhq/devvm/internal/clipboard"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/proxy"
	"github.com/devvmhq/devvm/internal/state"
	"github.com/devvmhq/devvm/internal/vmssh"
)

// Clipboard returns whether the folder's VM reads the image on this
// machine's clipboard: images or off.
func (e Settings) Clipboard(ctx context.Context) (string, error) {
	t, err := e.finder.target(ctx, need{has: hasMade, noVM: msg.NoVMClipboard})
	if err != nil {
		return "", err
	}
	return clipboard.Mode(t.st), nil
}

// SetClipboard turns clipboard images on (images) or off for the folder's
// VM. A running VM has the change at once (linkRuns.apply): its setup sent
// again links xclip and wl-paste to devvm-clip or removes them, and its
// link session listens for them, or stops.
func (e Settings) SetClipboard(ctx context.Context, mode string) error {
	t, err := e.finder.target(ctx, need{has: hasMade, noVM: msg.NoVMClipboard})
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	if clipboard.Mode(st) == mode {
		e.Info(msg.SettingAlready("devvm clipboard "+mode, vm))
		return nil
	}
	on := mode == clipboard.Images
	if on {
		if err := e.Registry.Require(b, vm, linkFeatures()...); err != nil {
			return err
		}
		if ok, err := t.widen("devvm clipboard " + mode); !ok {
			return err
		}
	}
	from := e.linkRuns.logSize(vm)
	if err := st.Update(ctx, func(st *state.VM) error { return clipboard.SetMode(st, mode) }); err != nil {
		return err
	}
	if on {
		e.Warn(msg.ClipboardOnRisk(vm))
		e.warnNoClipboardReader(vm)
	}
	running, err := e.linkRuns.apply(ctx, b, st, nil, on, from)
	switch {
	case err != nil:
		return err
	case running:
		e.Info(msg.SettingApplied(vm))
	default:
		e.Info(msg.SettingSet(vm))
	}
	return nil
}

// warnNoClipboardReader warns when this machine has nothing to read its
// clipboard with, as vm's images would come from: no display, or no
// xclip, wl-paste or osascript.
func (e *Env) warnNoClipboardReader(vm string) {
	if platform.ClipboardReader(e.Getenv) != nil {
		e.Warn(msg.ClipboardNoReader(vm))
	}
}

// clipboardLink is the link a session carries for a VM with clipboard
// images: devvm's own, on proxy.ClipboardAddr in the VM, each connection
// answered by srv.
func clipboardLink(srv *clipboard.Server) vmssh.Link {
	return vmssh.Link{
		Name: "clipboard", Addr: netip.MustParseAddr(proxy.ClipboardAddr), Port: clipboard.Port,
		Serve: func(ctx context.Context, conn net.Conn) { srv.Serve(ctx, conn) },
	}
}

// clipboardGuest is DEVVM_CLIPBOARD, where the VM's devvm-clip asks for the
// image, "ADDRESS PORT", for a VM with clipboard images; "" for one
// without.
func clipboardGuest(st *state.VM) string {
	if clipboard.Mode(st) != clipboard.Images {
		return ""
	}
	return proxy.ClipboardAddr + " " + strconv.Itoa(clipboard.Port)
}
