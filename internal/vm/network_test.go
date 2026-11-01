package vm

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// devvm network: shown as devvm ls shows it, then the VM's allowlist; on
// and off change the state only, applied when the VM next starts. A running VM turned on isn't
// confined, so its next launch offers the restart; one turned off stays
// on its proxy until it starts again.
func TestNetwork(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	n, l := Net{e}, Launch{e}
	shown := func(want string) {
		t.Helper()
		var b strings.Builder
		err := n.Network(t.Context(), &b)
		line, rules, _ := strings.Cut(b.String(), "\n")
		if err != nil || line != want || !strings.HasPrefix(rules, "# ") || !strings.Contains(rules, "/layers/base.policy\n  ") {
			t.Errorf("Network = %q, %v; want %s", b.String(), names(err), want)
		}
	}
	shown("off")
	for range 2 { // the second time it's already on
		if err := n.SetNetwork(t.Context(), true); err != nil {
			t.Fatal(names(err))
		}
	}
	if st, _ := state.ReadVM(e.Dirs, vm); st.Get(KeyNet) != "on" || slices.Contains(f.Methods(), "Stop") {
		t.Errorf("net=%q, calls %q", st.Get(KeyNet), f.Methods())
	}
	shown(netpolicy.CheckUnenforced)
	// Its next launch, without a terminal to offer the restart in, stops
	// it; the one after starts it confined.
	if err := l.Up(t.Context(), Options{}); names(err) != "net_unenforced_stop vm="+vm {
		t.Fatalf("Up = %v", names(err))
	}
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetProxy {
		t.Errorf("started %+v", c)
	}
	shown("on")
	// Off while it runs confined: it keeps its proxy until it starts again.
	if err := n.SetNetwork(t.Context(), false); err != nil {
		t.Fatal(names(err))
	}
	pid, ok := e.Proxy.PID(vm)
	before := len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if again, _ := e.Proxy.PID(vm); !ok || again != pid || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("proxy %d, was %d; calls %q", again, pid, f.Methods()[before:])
	}
	shown("off")
	if err := l.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetDirect {
		t.Errorf("restarted %+v", c)
	}
	if _, ok := e.Proxy.PID(vm); ok {
		t.Error("its proxy still runs")
	}
}

// A backend that can't confine a VM to its proxy refuses devvm network
// on, changing nothing.
func TestNetworkRequiresEgressProxy(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureEgressProxy] = backend.Emulated(false)
	e, _, _ := upEnv(t, caps)
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	err := Net{e}.SetNetwork(t.Context(), true)
	if got := names(err); got != "backend_unreviewed backend=fake feature=egress-proxy vm="+st.Name() {
		t.Errorf("Network on = %v", got)
	}
	if st, _ := state.ReadVM(e.Dirs, st.Name()); st.Get(KeyNet) != "off" {
		t.Errorf("net=%q", st.Get(KeyNet))
	}
}
