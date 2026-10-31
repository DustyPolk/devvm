package docker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/backend"
)

var (
	_ backend.Locator     = (*Docker)(nil)
	_ backend.Forgetter   = (*Docker)(nil)
	_ backend.StrayFinder = (*Docker)(nil)
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
// longer has, as Delete does after removing it: its port forwarder's
// files, its relay and networks, its ssh key and host keys, and its
// volumes (its home, a --clone VM's clone, the docker layer's engine
// files), each only if it has devvm's label, and the image its last
// change was committed to. A VM whose container is there is refused.
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
	if err := d.Forwards.Remove(vm); err != nil {
		return err
	}
	if err := (egress{d}).forget(ctx, vm); err != nil {
		return err
	}
	for _, f := range []string{d.sshKey(vm), d.knownHosts(vm), d.Dirs.DockerVM(vm)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	for _, v := range append([]string{homeVolume(vm), cloneVolume(vm)}, engineVolumes(vm)...) {
		if err := d.removeVolume(ctx, v); err != nil {
			return err
		}
	}
	if r.Commit != "" {
		return d.removeImage(ctx, r.Commit)
	}
	return nil
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
		if _, err := os.Stat(d.Dirs.VMState(in.Name)); errors.Is(err, fs.ErrNotExist) {
			out = append(out, backend.Stray{Name: in.Name, Data: d.Data})
		}
	}
	return out, nil
}
