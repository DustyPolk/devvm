package vm

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// layerFake is a backend that makes VMs for the docker layer, under
// runtime ("" for none: its refusal then).
type layerFake struct {
	*backendtest.Fake
	runtime string
	set     *[]string // SetDockerLayer's calls, by VM, "-" before one turning it off
}

func (l layerFake) DockerLayer(vm string) (string, error) {
	if l.runtime == "" {
		return "", msg.DockerLayerNeedsVM(vm)
	}
	return l.runtime, nil
}

func (l layerFake) SetDockerLayer(_ context.Context, vm string, on bool) error {
	if !on {
		vm = "-" + vm
	}
	*l.set = append(*l.set, vm)
	return nil
}

// layerEnv is upEnv on a layerFake with caps, warnings and infos recorded
// by name.
func layerEnv(t *testing.T, caps backend.Caps, runtime string) (*Env, *backendtest.Fake, *[]string) {
	t.Helper()
	e, f, _ := upEnv(t, caps)
	reg, err := backend.NewRegistry(layerFake{f, runtime, &[]string{}})
	if err != nil {
		t.Fatal(err)
	}
	e.Registry = reg
	var said []string
	e.Warn = func(m msg.M) { said = append(said, m.Names()) }
	e.Info = func(m msg.M) { said = append(said, m.Names()) }
	return e, f, &said
}

// noLayerCaps are a backend's that can't run the docker layer here.
func noLayerCaps() backend.Caps {
	caps := allNative()
	delete(caps, backend.FeatureDockerLayer)
	return caps
}

func saidAny(all []string, prefix string) bool {
	return slices.ContainsFunc(all, func(s string) bool { return strings.HasPrefix(s, prefix) })
}

// setCalls are a layerEnv backend's SetDockerLayer calls.
func setCalls(e *Env) []string {
	b, _ := e.Registry.Lookup("fake")
	return *b.(layerFake).set
}

// Where the backend can't run it, refused with its own message, before
// anything is built; with network policy, refused too.
func TestUpDockerLayerRefused(t *testing.T) {
	e, f, _ := layerEnv(t, noLayerCaps(), "")
	err := Launch{e}.Up(t.Context(), Options{Image: "docker", Ports: "none"})
	if got := names(err); !strings.HasPrefix(got, "docker_layer_needs_vm vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	e2, _, _ := layerEnv(t, allNative(), "sysbox")
	err = Launch{e2}.Up(t.Context(), Options{Image: "docker", Ports: "none", NetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "net_docker_layer backend=fake vm=devvm-web-") {
		t.Errorf("with network policy: %v", got)
	}
	// Unreviewed, it's refused as any unreviewed emulation.
	caps := allNative()
	caps[backend.FeatureDockerLayer] = backend.Emulated(false)
	e3, _, _ := layerEnv(t, caps, "sysbox")
	err = Launch{e3}.Up(t.Context(), Options{Image: "docker", Ports: "none"})
	if got := names(err); !strings.HasPrefix(got, "backend_unreviewed backend=fake feature=docker-layer vm=devvm-web-") {
		t.Errorf("unreviewed: %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") {
		t.Errorf("calls = %q", ms)
	}
}

// Under the backend's runtime: made for it, said, recorded; no network
// policy, now or later; ls shows it.
func TestUpDockerLayerRuntime(t *testing.T) {
	e, f, said := layerEnv(t, allNative(), "sysbox")
	if err := (Launch{e}).Up(t.Context(), Options{Image: "docker", Ports: "none"}); err != nil {
		t.Fatal(names(err))
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	if !spec.DockerLayer {
		t.Errorf("spec = %+v", spec)
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetDirect {
		t.Errorf("Start confinement = %+v", c)
	}
	if !saidAny(*said, "docker_layer_runtime backend=fake runtime=sysbox vm=") {
		t.Errorf("said %q", *said)
	}
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if st.Get(KeyDockerLayer) != "sysbox" || st.Get(KeyNet) != "off" {
		t.Errorf("state docker_layer=%q net=%q", st.Get(KeyDockerLayer), st.Get(KeyNet))
	}
	if err := (Net{e}).SetNetwork(t.Context(), true); !strings.HasPrefix(names(err), "net_docker_layer backend=fake vm=") {
		t.Errorf("network on: %v", names(err))
	}
	// A state edited by hand: refused all the same.
	if err := st.Set(KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); !strings.HasPrefix(names(err), "net_docker_layer backend=fake vm=") {
		t.Errorf("net=on in its state: %v", names(err))
	}
	if ls, err := List(t.Context(), e.Dirs, e.Registry); err != nil || len(ls) != 1 || ls[0].DockerLayer != "sysbox" {
		t.Errorf("List = %+v, %v", ls, err)
	}
}

// The docker layer added in place makes the VM one for it, as a new one's
// would be: stopped, set, recorded, then its script run; refused where
// the backend can't, and for a VM with network policy, before anything
// changes.
func TestAddDockerLayerInPlace(t *testing.T) {
	e, f, said := layerEnv(t, allNative(), "sysbox")
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, Ports: "none"})
	answers(t, e, true)
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{Image: "docker"}); err != nil {
		t.Fatal(names(err))
	}
	vm := vmName(t, e)
	st, _ := state.ReadVM(e.Dirs, vm)
	ms := f.Methods()[before:]
	if got := setCalls(e); !slices.Equal(got, []string{vm}) || !slices.Contains(ms, "Stop") || !slices.Contains(ms, "Exec") ||
		st.Get(KeyDockerLayer) != "sysbox" || st.Get(state.KeyImage) != "docker" || !saidAny(*said, "docker_layer_runtime backend=fake runtime=sysbox vm=") {
		t.Errorf("set %q, calls %q, docker_layer=%q image=%q, said %q", got, ms, st.Get(KeyDockerLayer), st.Get(state.KeyImage), *said)
	}

	// Its script failing: the VM made one as it was, stopped first, its
	// state as before; a launch after that may turn network policy on.
	ef, ff, _ := layerEnv(t, allNative(), "sysbox")
	stf := existingVM(t, ef, Options{Image: "base", NoNetworkPolicy: true, Ports: "none"})
	answers(t, ef, true)
	ff.ExecFunc = func(_ string, argv []string, _ backend.ExecOpts) error {
		if slices.Equal(argv, []string{"bash", "-s"}) {
			return errors.New("exit 1")
		}
		return nil
	}
	before = len(ff.Calls())
	if err := (Launch{ef}).Up(t.Context(), Options{Image: "docker"}); !strings.HasPrefix(names(err), "add_layer_failed ") {
		t.Errorf("failing script: %v", names(err))
	}
	stf, _ = state.ReadVM(ef.Dirs, stf.Name())
	ms = ff.Methods()[before:]
	if got := setCalls(ef); !slices.Equal(got, []string{stf.Name(), "-" + stf.Name()}) || ms[len(ms)-1] != "Stop" ||
		stf.Get(KeyDockerLayer) != "" || stf.Get(state.KeyImage) != "base" {
		t.Errorf("after a failed script: set %q, calls %q, docker_layer=%q image=%q", got, ms, stf.Get(KeyDockerLayer), stf.Get(state.KeyImage))
	}
	if err := (Net{ef}).SetNetwork(t.Context(), true); err != nil {
		t.Errorf("network on: %v", names(err))
	}

	e2, f2, _ := layerEnv(t, noLayerCaps(), "")
	existingVM(t, e2, Options{Image: "base", NoNetworkPolicy: true, Ports: "none"})
	answers(t, e2, true)
	before = len(f2.Calls())
	err := Launch{e2}.Up(t.Context(), Options{Image: "docker"})
	if got := names(err); !strings.HasPrefix(got, "docker_layer_needs_vm vm=devvm-web-") {
		t.Errorf("where it can't: %v", got)
	}
	if ms := f2.Methods()[before:]; slices.Contains(ms, "Stop") || slices.Contains(ms, "Exec") || len(setCalls(e2)) > 0 {
		t.Errorf("calls = %q", ms)
	}

	e3, f3, _ := layerEnv(t, allNative(), "sysbox")
	st3 := existingVM(t, e3, Options{Image: "base", NoNetworkPolicy: true, Ports: "none"})
	answers(t, e3, true)
	if err := st3.Set(KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	before = len(f3.Calls())
	err = Launch{e3}.Up(t.Context(), Options{Image: "docker"})
	if got := names(err); !strings.HasPrefix(got, "net_docker_layer backend=fake vm=devvm-web-") {
		t.Errorf("with network policy: %v", got)
	}
	if ms := f3.Methods()[before:]; slices.Contains(ms, "Stop") || len(setCalls(e3)) > 0 {
		t.Errorf("calls = %q", ms)
	}
}
