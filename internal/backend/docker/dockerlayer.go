package docker

// The docker layer (Docker's engine in a VM), emulated, reviewed, under
// Sysbox alone (FeatureDockerLayer), where a Linux engine of this
// machine's has its runtime and its bridges don't route to the loopback: a
// container runs dockerd only with more than devvm's others get, so a VM
// is made for it (backend.DockerLayerVM).
//
// Sysbox gives the container a user namespace of its own (root in it is an
// unprivileged uid range on the host, but on its mounts, ID-mapped, where
// it writes as the host's root, as in any Docker VM) and emulates /proc
// and /sys, without --privileged, which devvm never uses: on Linux that's
// the host's root, on Docker Desktop root of its VM with the folders it
// shares. Sysbox gives root in it every capability within that namespace,
// --cap-drop aside, so it has no NET_RAW drop.
//
// It's on a network of its own, not internal and with nothing else on it,
// rather than Docker's default one, so what root in it sends reaches no
// other container (layerNetwork); its engine's files are in volumes of its
// own (engineVolumes: overlayfs doesn't stack on the container's own, and
// they outlive its container being made again, start.go); services.sh
// starts dockerd after each start; and it gets no network policy: root in
// it can change its own network (start.go, and devvm's callers, refuse
// it).

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// Where Docker's engine keeps its files in a VM.
const (
	dockerDataAt     = "/var/lib/docker"
	containerdDataAt = "/var/lib/containerd"
)

// engineVolumes are the volumes of a VM made for the docker layer, for
// dockerDataAt and containerdDataAt, in that order.
func engineVolumes(vm string) []string { return []string{vm + "-docker", vm + "-containerd"} }

// hasSysbox reports whether the engine Problems last found has Sysbox's
// runtime; false before Problems looked.
func hasSysbox(d *Docker) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.eng == nil {
		return false
	}
	_, ok := d.eng.Runtimes[sysboxRuntime]
	return ok
}

// DockerLayer is sysbox where the engine, a Linux one of this machine's
// (whose bridges' settings devvm reads, loopbackClosed), has Sysbox's
// runtime, which VMs with the docker layer run under. Elsewhere it's
// refused: on a Linux engine of this machine's, saying to install Sysbox or
// use a VM backend; on any other (Docker Desktop's), to use a VM backend.
func (d *Docker) DockerLayer(vm string) (string, error) {
	switch {
	case d.engineKind() != kindLinux:
		return "", msg.DockerLayerNeedsVM(vm)
	case !hasSysbox(d):
		return "", msg.DockerLayerNoSysbox(vm)
	}
	return "sysbox", nil
}

// layerNetName is a VM's own network, made for the docker layer.
func layerNetName(vm string) string { return vm + "-docker-net" }

// directNet is the network a VM is on without network policy: Docker's
// default one, or for the docker layer its own (layerNetwork).
func directNet(vm string, r record) string {
	if r.dockerLayer() {
		return layerNetName(vm)
	}
	return "bridge"
}

// layerNetwork readies a docker layer VM's own network before its
// container is made or started: devvm's, labelled, not internal (it
// reaches the internet), with nothing else on it, so that the frames root
// in the VM can send reach no other container. One that isn't devvm's is
// refused, and so is one with a container on it (the VM is stopped),
// naming it.
func (e egress) layerNetwork(ctx context.Context, vm string) error {
	name := layerNetName(vm)
	if _, err := e.ensureNetwork(ctx, name, false); err != nil {
		return err
	}
	n, err := e.network(ctx, name)
	if err != nil {
		return err
	}
	if len(n.Containers) > 0 {
		return msg.DockerNetworkShared(name)
	}
	return e.loopbackClosed(vm, n)
}

// loopbackClosed refuses a docker layer VM whose network's bridge (n, a
// plain one, named for its ID) has route_localnet on, as Docker sets it
// with its userland proxy off: root in the VM, routing 127.0.0.1 to its
// gateway, would reach this machine's loopback. One whose setting can't be
// read is refused too.
func (e egress) loopbackClosed(vm string, n network) error {
	bridge := "br-" + n.ID[:min(12, len(n.ID))]
	b, err := os.ReadFile(filepath.Join(e.d.ProcSys, "net", "ipv4", "conf", bridge, "route_localnet")) //nolint:gosec // the kernel's setting for devvm's own bridge
	switch {
	case err != nil:
		return msg.DockerBridgeUnreadable(vm, bridge).Because(err)
	case strings.TrimSpace(string(b)) != "0":
		return msg.DockerRouteLocalnet(vm)
	}
	return nil
}

// layerConfig is c for a VM made for the docker layer (r): on its own
// network (layerNetName), on its runtime, with no capability dropped
// (Sysbox drops none), and its engine's volumes; c as it is for any other.
func layerConfig(c config, vm string, r record) config {
	if !r.dockerLayer() {
		return c
	}
	c.Net, c.Runtime, c.CapDrop = layerNetName(vm), r.Runtime, nil
	vols := engineVolumes(vm)
	c.Mounts = append(c.Mounts, volume(vols[0], dockerDataAt, true), volume(vols[1], containerdDataAt, true))
	return c
}
