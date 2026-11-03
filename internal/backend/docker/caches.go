package docker

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/paths"
)

var _ backend.CacheCleaner = (*Docker)(nil)

// DevvmCache lists devvm's volumes in the engine that no VM of devvm's
// has: those with its label that aren't a listed VM's own (its home, a
// --clone VM's clone, the docker layer's engine files) and aren't the
// shared tools' volume, such as the home of a VM docker rm deleted.
// Anything without devvm's label is never one, and Docker is never
// pruned.
func (d *Docker) DevvmCache(ctx context.Context) ([]backend.CacheEntry, error) {
	free, err := freeVolumes(ctx, d)
	if err != nil {
		return nil, err
	}
	sizes, err := d.volumeSizes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]backend.CacheEntry, 0, len(free))
	for _, v := range free {
		out = append(out, backend.CacheEntry{What: "volume " + v, Name: v, Size: sizes[v]})
	}
	return out, nil
}

// DeleteCache deletes volumes DevvmCache listed that still are devvm's
// and no VM's.
func (d *Docker) DeleteCache(ctx context.Context, entries []backend.CacheEntry) error {
	free, err := freeVolumes(ctx, d)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name != "" && slices.Contains(free, e.Name) {
			if err := d.removeVolume(ctx, e.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

// freeVolumes are devvm's labelled volumes that no listed VM has and
// that aren't the shared tools'.
func freeVolumes(ctx context.Context, d *Docker) ([]string, error) {
	vms, err := d.List(ctx)
	if err != nil {
		return nil, err
	}
	owned := map[string]bool{d.toolsVolume(): true}
	for _, in := range vms {
		for _, v := range append([]string{homeVolume(in.Name), cloneVolume(in.Name)}, engineVolumes(in.Name)...) {
			owned[v] = true
		}
	}
	ours, err := d.volumes(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range ours {
		if !owned[v] && !hasState(d.Dirs, v) {
			out = append(out, v)
		}
	}
	return out, nil
}

// hasState reports whether a volume is one of a VM's that devvm has
// state for: a VM being made has its volumes before its container, and
// one whose container is gone is gc --orphans's.
func hasState(dirs paths.Dirs, volume string) bool {
	for _, suffix := range []string{"-home", "-clone", "-docker", "-containerd"} {
		if vm, ok := strings.CutSuffix(volume, suffix); ok {
			if _, err := os.Stat(dirs.VM(vm).State()); err == nil {
				return true
			}
		}
	}
	return false
}
