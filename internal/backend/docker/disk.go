package docker

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// Docker sets no size on a container's disk: Docker Desktop refuses one,
// and the containerd image store, Linux's default, ignores it. So a VM's
// disk is the engine's, and devvm says what it holds instead (DiskUsed):
// what its container wrote and its volumes (its home, a --clone VM's
// clone, the docker layer's engine), against base.yaml's disk, the size
// VMs get on other backends.

// DiskUsed is what the VM's container and its volumes hold, in bytes, and
// base's disk size.
func (d *Docker) DiskUsed(ctx context.Context, vm string) (used, limit int64, err error) {
	if _, err := d.vm(ctx, vm); err != nil {
		return 0, 0, err
	}
	_, _, disk, err := d.baseSize()
	if err != nil {
		return 0, 0, err
	}
	sizes, err := d.volumeSizes(ctx)
	if err != nil {
		return 0, 0, err
	}
	ours, err := d.volumes(ctx)
	if err != nil {
		return 0, 0, err
	}
	used, _, err = d.vmDisk(ctx, vm, sizes, ours)
	return used, int64(disk * gib), err
}

// vmDisk is what a VM holds, for devvm size, devvm space and the warning
// past base's disk alike: what its container wrote and its own volumes
// with devvm's label (ours), by sizes (volumeSizes). It also returns
// those volumes.
func (cl client) vmDisk(ctx context.Context, vm string, sizes map[string]int64, ours []string) (int64, []string, error) {
	out, err := cl.Runner.Output(cl.docker(ctx, "container", "inspect", "--size", "--format", "{{.SizeRw}}", vm))
	if err != nil {
		return 0, nil, fmt.Errorf("docker container inspect --size: %w", err)
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("docker container inspect --size: %w", err)
	}
	var vols []string
	for _, v := range append([]string{homeVolume(vm), cloneVolume(vm)}, engineVolumes(vm)...) {
		if slices.Contains(ours, v) {
			n += sizes[v]
			vols = append(vols, v)
		}
	}
	return n, vols, nil
}

// parseSize reads a size as docker system df shows it (3.238kB, 1.2GB,
// 0B), in bytes; 0 for what it can't read.
func parseSize(s string) int64 {
	units := []struct {
		suffix string
		mult   float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}}
	for _, u := range units {
		if n, ok := strings.CutSuffix(s, u.suffix); ok {
			f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err != nil || f < 0 || math.IsInf(f, 0) {
				return 0
			}
			return int64(f * u.mult)
		}
	}
	return 0
}
