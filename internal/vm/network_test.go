package vm

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
)

// setNetwork is devvm network on (allowlist) or off (open).
func setNetwork(ctx context.Context, n Net, on bool) error {
	if on {
		return n.SetTier(ctx, netpolicy.TierAllowlist, "devvm network on")
	}
	return n.SetTier(ctx, netpolicy.TierOpen, "devvm network off")
}

// devvm network: shown as devvm ls shows it, then the VM's allowlist; on
// and off change the state only, applied when the VM next starts. A running VM turned on isn't
// confined, so its next launch offers the restart; one turned off stays
// on its proxy until it starts again.
func TestNetwork(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	n, l := Net{e}, Launch{e}
	shown := func(want string) {
		t.Helper()
		s, err := n.Network(t.Context())
		// Open has no rules.
		rules := want == "open" && len(s.Rules) == 0 ||
			len(s.Rules) > 0 && s.Rules[0].Path == "layers/base.policy" && s.Rules[0].Builtin && len(s.Rules[0].Rules()) > 0
		if err != nil || s.Policy != want || !rules {
			t.Errorf("Network = %+v, %v; want %s", s, msg.NamesOf(err), want)
		}
	}
	shown("open")
	for range 2 { // the second time it's already on
		if err := setNetwork(t.Context(), n, true); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
	}
	if st, _ := state.ReadVM(e.Dirs, vm); get(st, KeyNet) != "allowlist" || slices.Contains(f.Methods(), "Stop") {
		t.Errorf("net=%q, calls %q", get(st, KeyNet), f.Methods())
	}
	shown(netpolicy.CheckUnenforced)
	// Its next launch, without a terminal to offer the restart in, stops
	// it; the one after starts it confined.
	if err := l.Up(t.Context(), Options{}); msg.NamesOf(err) != "net_unenforced_stop vm="+vm {
		t.Fatalf("Up = %v", msg.NamesOf(err))
	}
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if c, _ := call(f, "Start").Args[2].(backend.Confinement); c.Net != backend.NetProxy {
		t.Errorf("started %+v", c)
	}
	shown("allowlist")
	// Off while it runs confined: it stays confined until it starts again,
	// its proxy started again as open's.
	if err := setNetwork(t.Context(), n, false); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	before := len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	cfg, err := proxy.ReadConfig(e.Dirs.VM(vm).ProxyConfig())
	if _, ok := e.Proxy.PID(vm); !ok || err != nil || !cfg.Open || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("proxy running %v, open %v (%v); calls %q", ok, cfg.Open, err, f.Methods()[before:])
	}
	shown("open")
	if err := l.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	// Open: its backend's own network, with its proxy set in it.
	if c, _ := call(f, "Start").Args[2].(backend.Confinement); c.Net != backend.NetDirect || !c.GuestProxy() {
		t.Errorf("restarted %+v", c)
	}
	if _, ok := e.Proxy.PID(vm); !ok {
		t.Error("its proxy doesn't run")
	}
}

// Between confined tiers the change applies at once: a running VM's
// proxy starts again with the new tier's rules, and nothing restarts.
// A lower tier needs the widening check.
func TestSetTierConfined(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{NetworkPolicy: true})
	vm, n := st.Name(), Net{e}
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	pid, _ := e.Proxy.PID(vm)
	before := len(f.Calls())
	if err := n.SetTier(t.Context(), netpolicy.TierNone, "devvm network none"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	again, ok := e.Proxy.PID(vm)
	if !ok || again == pid || slices.Contains(f.Methods()[before:], "Stop") || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("proxy %d, was %d; calls %q", again, pid, f.Methods()[before:])
	}
	if cfg, err := proxy.ReadConfig(e.Dirs.VM(vm).ProxyConfig()); err != nil || len(cfg.DenyPolicies) == 0 {
		t.Errorf("none's proxy: %+v, %v", cfg, err)
	}
	if st, _ := state.ReadVM(e.Dirs, vm); get(st, KeyNet) != "none" {
		t.Errorf("net=%q", get(st, KeyNet))
	}
	// Lower, to internet: widening, which a VM in a terminal of its own
	// would be asked; here it's let through.
	if err := n.SetTier(t.Context(), netpolicy.TierInternet, "devvm network internet"); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ := state.ReadVM(e.Dirs, vm); get(st, KeyNet) != "internet" {
		t.Errorf("net=%q", get(st, KeyNet))
	}
}

// A backend that can't confine a VM to its proxy refuses devvm network
// on, changing nothing.
func TestNetworkRequiresEgressProxy(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureEgressProxy] = backend.Emulated(false)
	e, _, _ := upEnv(t, caps)
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	err := setNetwork(t.Context(), Net{e}, true)
	if got := msg.NamesOf(err); got != "backend_unreviewed backend=fake feature=egress-proxy vm="+st.Name() {
		t.Errorf("Network on = %v", got)
	}
	if st, _ := state.ReadVM(e.Dirs, st.Name()); get(st, KeyNet) != "open" {
		t.Errorf("net=%q", get(st, KeyNet))
	}
}
