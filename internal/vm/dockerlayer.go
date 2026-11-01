package vm

import (
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// dockerLayers is how new VMs run the docker layer on a backend that
// makes VMs for it (backend.DockerLayerVM): the runtime (KeyDockerLayer).
// Such a VM gets no network policy.
type dockerLayers struct{ *Env }

// choice is the runtime a VM with the docker layer runs it under on a
// backend that makes VMs for it (dl), and the features its layers (img)
// need (docker-layer, refused by Require while unreviewed, and any
// other's); the backend's refusal where it can't run it.
func (e dockerLayers) choice(dl backend.DockerLayerVM, vm, img string) (string, []backend.Feature, error) {
	rt, err := dl.DockerLayer(vm)
	if err != nil {
		return "", nil, err
	}
	return rt, backend.ImageFeatures(img), nil
}

// say says which runtime runs a VM's docker layer; nothing for "".
func (e dockerLayers) say(b backend.Backend, mode, vm string) {
	if mode != "" {
		e.Info(msg.DockerLayerRuntime(b.Name(), mode, vm))
	}
}
