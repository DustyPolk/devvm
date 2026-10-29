package vm

import (
	"context"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// dockerLayers is how VMs run the docker layer on a backend that makes
// VMs for it (backend.DockerLayerVM), new ones and those it's added to in
// place alike: the runtime (KeyDockerLayer), and setting it on a VM that
// exists. Such a VM gets no network policy.
type dockerLayers struct{ *Env }

// choice is the runtime a VM with the docker layer runs it under on a
// backend that makes VMs for it (dl), and the feature that needs
// (docker-layer, refused by Require while unreviewed); the backend's
// refusal where it can't run it.
func (e dockerLayers) choice(dl backend.DockerLayerVM, vm string) (string, []backend.Feature, error) {
	rt, err := dl.DockerLayer(vm)
	if err != nil {
		return "", nil, err
	}
	return rt, []backend.Feature{backend.FeatureDockerLayer}, nil
}

// say says which runtime runs a VM's docker layer; nothing for "".
func (e dockerLayers) say(b backend.Backend, mode, vm string) {
	if mode != "" {
		e.Info(msg.DockerLayerRuntime(b.Name(), mode, vm))
	}
}

// inPlace checks the docker layer added to a VM in place (when added has
// it) on a backend that makes VMs for it: its runtime (choice), or the
// backend's refusal; refused too for a VM with network policy, before
// anything changes. It returns the runtime ("" otherwise) and what adding
// the layers needs of the backend.
func (e dockerLayers) inPlace(b backend.Backend, st *state.VM, added string) (string, []backend.Feature, error) {
	dl, made := b.(backend.DockerLayerVM)
	if !made || !image.HasLayer(added, image.DockerLayer) {
		return "", backend.ImageFeatures(added), nil
	}
	mode, need, err := e.choice(dl, st.Name())
	if err == nil && netpolicy.On(st) {
		err = msg.NetDockerLayer(b.Name(), st.Name())
	}
	return mode, need, err
}

// set makes a stopped VM one for the docker layer under runtime mode,
// recorded (KeyDockerLayer), its backend's container made again at its
// next start; nothing for "".
func (e dockerLayers) set(ctx context.Context, b backend.Backend, st *state.VM, mode string) error {
	dl, made := b.(backend.DockerLayerVM)
	if mode == "" || !made {
		return nil
	}
	if err := dl.SetDockerLayer(ctx, st.Name(), true); err != nil {
		return err
	}
	if err := st.Set(KeyDockerLayer, mode); err != nil {
		return err
	}
	e.say(b, mode, st.Name())
	return nil
}

// unset makes a VM that set made one for the docker layer under mode one
// as it was, once the layer's script failed: stopped, its backend's
// container made again without it, its state's KeyDockerLayer emptied.
// Nothing for "". It runs on when ctx is cancelled (an interrupted
// script), which would leave the VM without network policy for good.
func (e dockerLayers) unset(ctx context.Context, b backend.Backend, st *state.VM, mode string) error {
	dl, made := b.(backend.DockerLayerVM)
	if mode == "" || !made {
		return nil
	}
	ctx = context.WithoutCancel(ctx)
	if err := b.Stop(ctx, st.Name()); err != nil {
		return err
	}
	if err := dl.SetDockerLayer(ctx, st.Name(), false); err != nil {
		return err
	}
	return st.Set(KeyDockerLayer, "")
}
