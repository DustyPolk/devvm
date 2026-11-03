package vm

import (
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
}

func (l layerFake) DockerLayer(vm string) (string, error) {
	if l.runtime == "" {
		return "", msg.DockerLayerNeedsVM(vm)
	}
	return l.runtime, nil
}

// layerEnv is upEnv on a layerFake with caps, warnings and infos recorded
// by name.
func layerEnv(t *testing.T, caps backend.Caps, runtime string) (*Env, *backendtest.Fake, *msg.Recorder) {
	t.Helper()
	e, f, _ := upEnv(t, caps)
	reg, err := backend.NewRegistry(layerFake{f, runtime})
	if err != nil {
		t.Fatal(err)
	}
	reg.DefaultBackend = fakeDefault
	e.Registry = reg
	said := &msg.Recorder{}
	e.Warn = said.Add
	e.Info = said.Add
	return e, f, said
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

// Where the backend can't run it, refused with its own message, before
// anything is built; with network policy, refused too.
func TestUpDockerLayerRefused(t *testing.T) {
	e, f, _ := layerEnv(t, noLayerCaps(), "")
	err := Launch{e}.Up(t.Context(), Options{Layers: &[]string{"docker"}, Ports: "none"})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "docker_layer_needs_vm vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	e2, _, _ := layerEnv(t, allNative(), "sysbox")
	err = Launch{e2}.Up(t.Context(), Options{Layers: &[]string{"docker"}, Ports: "none", NetworkPolicy: true})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "net_docker_layer backend=fake vm=devvm-web-") {
		t.Errorf("with network policy: %v", got)
	}
	// Unreviewed, it's refused as any unreviewed emulation.
	caps := allNative()
	caps[backend.FeatureDockerLayer] = backend.Emulated(false)
	e3, _, _ := layerEnv(t, caps, "sysbox")
	err = Launch{e3}.Up(t.Context(), Options{Layers: &[]string{"docker"}, Ports: "none"})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_unreviewed backend=fake feature=docker-layer vm=devvm-web-") {
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
	if err := (Launch{e}).Up(t.Context(), Options{Layers: &[]string{"docker"}, Ports: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	if !spec.DockerLayer {
		t.Errorf("spec = %+v", spec)
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.Net != backend.NetDirect {
		t.Errorf("Start confinement = %+v", c)
	}
	if !saidAny(said.Names(), "docker_layer_runtime backend=fake runtime=sysbox vm=") {
		t.Errorf("said %q", said.Names())
	}
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if st.Get(KeyDockerLayer) != "sysbox" || st.Get(KeyNet) != "off" {
		t.Errorf("state docker_layer=%q net=%q", st.Get(KeyDockerLayer), st.Get(KeyNet))
	}
	if err := (Net{e}).SetNetwork(t.Context(), true); !strings.HasPrefix(msg.NamesOf(err), "net_docker_layer backend=fake vm=") {
		t.Errorf("network on: %v", msg.NamesOf(err))
	}
	// A state edited by hand: refused all the same.
	if err := st.Set(KeyNet, "on"); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); !strings.HasPrefix(msg.NamesOf(err), "net_docker_layer backend=fake vm=") {
		t.Errorf("net=on in its state: %v", msg.NamesOf(err))
	}
	if ls, err := List(t.Context(), e.Dirs, e.Registry); err != nil || len(ls) != 1 || ls[0].DockerLayer != "sysbox" {
		t.Errorf("List = %+v, %v", ls, err)
	}
}
