package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
)

var _ backend.UsageReporter = (*Docker)(nil)

// Usage is what each VM's container wrote and its own volumes hold, and
// devvm's other volumes in the engine (the shared tools', and those of
// VMs whose container is gone), by their labels: never anyone else's.
func (d *Docker) Usage(ctx context.Context) (backend.Usage, error) {
	u := backend.Usage{VMs: map[string]int64{}}
	vms, err := d.List(ctx)
	if err != nil {
		return u, err
	}
	sizes, err := d.volumeSizes(ctx)
	if err != nil {
		return u, err
	}
	ours, err := d.volumes(ctx)
	if err != nil {
		return u, err
	}
	owned := map[string]bool{}
	for _, in := range vms {
		out, err := d.Runner.Output(d.docker(ctx, "container", "inspect", "--size", "--format", "{{.SizeRw}}", in.Name))
		if err != nil {
			continue
		}
		n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			continue
		}
		for _, v := range append([]string{homeVolume(in.Name), cloneVolume(in.Name)}, engineVolumes(in.Name)...) {
			if slices.Contains(ours, v) {
				n += sizes[v]
				owned[v] = true
			}
		}
		u.VMs[in.Name] = n
	}
	c := backend.Cache{Name: "volumes", Sized: true}
	for _, v := range ours {
		if !owned[v] {
			c.Count++
			c.Size += sizes[v]
		}
	}
	u.Caches = append(u.Caches, c)
	return u, nil
}

// volumes lists the volumes with this devvm's label.
func (cl client) volumes(ctx context.Context) ([]string, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "volume", "ls", "--filter", "label="+LabelKey+"="+cl.Data, "--format", "{{.Name}}"))
	if err != nil {
		return nil, fmt.Errorf("docker volume ls: %w", err)
	}
	return strings.Fields(string(out)), nil
}

// volumeSizes is what each volume in the engine holds, as docker system
// df says, by name.
func (cl client) volumeSizes(ctx context.Context) (map[string]int64, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "system", "df", "-v", "--format", "{{json .Volumes}}"))
	if err != nil {
		return nil, fmt.Errorf("docker system df: %w", err)
	}
	var vols []struct{ Name, Size string }
	if err := json.Unmarshal(out, &vols); err != nil {
		return nil, fmt.Errorf("docker system df: %w", err)
	}
	sizes := map[string]int64{}
	for _, v := range vols {
		sizes[v.Name] = parseSize(v.Size)
	}
	return sizes, nil
}
