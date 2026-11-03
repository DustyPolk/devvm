package docker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

var (
	_ backend.Locator        = (*Docker)(nil)
	_ backend.Forgetter      = (*Docker)(nil)
	_ backend.LeftoverLister = (*Docker)(nil)
	_ backend.StrayFinder    = (*Docker)(nil)
)

// Location is the engine this run's docker reaches (Docker Desktop's, a
// Linux engine, another context's): its ID and name as docker info has
// them once the backend has looked, else DOCKER_HOST and DOCKER_CONTEXT.
// Another engine has other containers. Every engine counts as the
// default, as devvm can't tell which one a VM that records none was
// made on.
func (d *Docker) Location() (string, bool) {
	d.mu.Lock()
	eng := d.eng
	d.mu.Unlock()
	if eng != nil && eng.ID+eng.Name != "" {
		return "engine\n" + eng.ID + "\n" + eng.Name, true
	}
	return "env\n" + d.Getenv("DOCKER_HOST") + "\n" + d.Getenv("DOCKER_CONTEXT"), true
}

// Forget deletes what devvm keeps for a VM whose container the engine no
// longer has, as Delete does after removing it: what it has in the engine
// (removeLeaves: its relay and networks, its volumes, each only if it has
// devvm's label, and the image its last change was committed to), then
// its backend folder, with its record, keys and port forwarder's files.
// A VM whose container is there is refused.
func (d *Docker) Forget(ctx context.Context, vm string) error {
	if _, err := d.vm(ctx, vm); err == nil {
		return fmt.Errorf("docker: %s exists", vm)
	} else if !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	r, err := d.readRecord(vm)
	if err != nil {
		return err
	}
	d.Forwards.Stop(vm)
	if err := d.removeLeaves(ctx, vm, r); err != nil {
		return err
	}
	return d.Forwards.Forget(vm)
}

// Leftovers are what Forget deletes of Docker's own for a VM, as the
// engine has them now: its relay, its networks and volumes, and the image
// its last change was committed to. Someone else's by one of those names
// fails it, as it does Forget.
func (d *Docker) Leftovers(ctx context.Context, vm string) ([]backend.Leftover, error) {
	var out []backend.Leftover
	relay := relayName(vm)
	if _, err := d.inspect(ctx, relay); err == nil {
		out = append(out, backend.Leftover{Kind: backend.LeftoverContainer, Name: relay})
	} else if errors.Is(err, errForeign) {
		return nil, msg.DockerContainerForeign(relay)
	} else if !errors.Is(err, backend.ErrNotFound) {
		return nil, err
	}
	for _, n := range []string{netName(vm), layerNetName(vm)} {
		if _, err := (egress{d}).network(ctx, n); err == nil {
			out = append(out, backend.Leftover{Kind: backend.LeftoverNetwork, Name: n})
		} else if errors.Is(err, errForeign) {
			return nil, msg.DockerNetworkForeign(n)
		} else if !errors.Is(err, backend.ErrNotFound) {
			return nil, err
		}
	}
	for _, v := range append([]string{homeVolume(vm), cloneVolume(vm)}, engineVolumes(vm)...) {
		if ok, err := d.volumeExists(ctx, v); err != nil {
			return nil, err
		} else if ok {
			out = append(out, backend.Leftover{Kind: backend.LeftoverVolume, Name: v})
		}
	}
	r, err := d.readRecord(vm)
	if err != nil || r.Commit == "" {
		return out, err
	}
	if _, err := d.inspectImage(ctx, r.Commit); err == nil {
		out = append(out, backend.Leftover{Kind: backend.LeftoverImage, Name: r.Commit})
	} else if !errors.Is(err, backend.ErrNotFound) {
		return nil, err
	}
	return out, nil
}

// Strays are the VMs' containers with this devvm's label that it has no
// state for.
func (d *Docker) Strays(ctx context.Context) ([]backend.Stray, error) {
	vms, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []backend.Stray
	for _, in := range vms {
		if _, err := os.Stat(d.Dirs.VM(in.Name).State()); errors.Is(err, fs.ErrNotExist) {
			out = append(out, backend.Stray{Name: in.Name, Data: d.Data})
		}
	}
	return out, nil
}
