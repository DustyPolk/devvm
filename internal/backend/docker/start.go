package docker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vmname"
)

// maxLayers is the most layers a VM's own commit has before devvm
// flattens it: overlayfs stacks at most about 125.
const maxLayers = 100

// config is what Docker fixes when it makes a container: what devvm
// compares at each start with what the VM wants.
type config struct {
	Hostname string
	// Net is the network it's made on: bridge, none or a build's own. A
	// VM's is bridge, and each start moves it where it goes (egress.go).
	Net    string
	Mounts []string // --mount's values, in order
	// CapDrop are the capabilities it's made without: a VM's, and a
	// build's on a network of its own, NET_RAW (no raw or packet sockets:
	// ping uses ICMP datagram sockets).
	CapDrop []string
	// Runtime is a VM's made for the docker layer: Sysbox's
	// (dockerlayer.go).
	Runtime string
}

// hash is the config's hash, which a VM's container carries (configLabel).
func (c config) hash() string {
	b, _ := json.Marshal(c) // strings only: it can't fail
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

// args are docker container create's options for the config.
func (c config) args() []string {
	args := []string{"--hostname", c.Hostname, "--network", c.Net}
	for _, m := range c.Mounts {
		args = append(args, "--mount", m)
	}
	for _, cp := range c.CapDrop {
		args = append(args, "--cap-drop", cp)
	}
	if c.Runtime != "" {
		args = append(args, "--runtime", c.Runtime)
	}
	return args
}

// netMode is Docker's network for a network devvm asks for: its default
// bridge, or none. A Linux engine's bridge reaches the internet and not
// the host's loopback; Docker Desktop's reaches both, so its builds get a
// network of their own (egress.go).
func netMode(n backend.Net) string {
	if n == backend.NetNone {
		return "none"
	}
	return "bridge"
}

// bind is a --mount value for a host folder at the same path in the
// container.
func bind(path string, writable bool) (string, error) { return bindAt(path, path, writable) }

// bindAt is a --mount value for a host folder at a path in the container.
// Docker reads it as CSV: a path with a comma, a quote or a newline is
// refused.
func bindAt(path, at string, writable bool) (string, error) {
	for _, p := range []string{path, at} {
		if strings.ContainsAny(p, ",\"\n") {
			return "", msg.DockerMountPath(p)
		}
	}
	m := "type=bind,src=" + path + ",dst=" + at
	if !writable {
		m += ",readonly"
	}
	return m, nil
}

// volume is a --mount value for a volume.
func volume(name, at string, writable bool) string {
	m := "type=volume,src=" + name + ",dst=" + at
	if !writable {
		m += ",readonly"
	}
	return m
}

// vmConfig is what the VM's container is made with: its hostname, from
// its name (devvm-<folder>-<hash>: the folder's part, as on Lima); Docker's
// default network; no NET_RAW; its mounts: the project folder (or its
// clone's volume), its agents' folders, the tools volume and its home's
// volume. One made for the docker layer differs (layerConfig).
func (d *Docker) vmConfig(vm string, r record) (config, error) {
	name, _, _, err := d.user()
	if err != nil {
		return config{}, err
	}
	c := config{Hostname: hostname(vm), Net: netMode(backend.NetDirect), CapDrop: []string{"NET_RAW"}}
	if r.Project != "" {
		m, err := bind(r.Project, true)
		if err != nil {
			return config{}, err
		}
		c.Mounts = append(c.Mounts, m)
	}
	if r.CloneAt != "" {
		c.Mounts = append(c.Mounts, volume(cloneVolume(vm), r.CloneAt, true))
	}
	for _, dir := range r.AgentDirs {
		m, err := bind(dir, false)
		if err != nil {
			return config{}, err
		}
		c.Mounts = append(c.Mounts, m)
	}
	if r.Tools {
		c.Mounts = append(c.Mounts, volume(d.toolsVolume(), toolsAt, false))
	}
	c.Mounts = append(c.Mounts, volume(homeVolume(vm), home(name), true))
	return layerConfig(c, vm, r), nil
}

// hostname is a VM's hostname: its folder's part of devvm-<folder>-<hash>,
// as devvm's setup would set it; the name itself for any other.
func hostname(vm string) string {
	if s, ok := vmname.SlugOf(vm); ok {
		return s
	}
	return vm
}

// noClientProxy are container create's and run's options, given to every
// container devvm makes (VMs, builds, relays, the tools' unpacking), that
// keep the proxies in
// the Docker client's config (~/.docker/config.json's proxies, password
// and all) out of a container's configuration: the client sets them in
// every container it makes unless the command sets them itself, and a
// commit keeps them, so a build's image would hold them, and a VM's
// processes could read them. devvm sets them empty instead; its own
// commands get the proxy they need (egress.go).
func noClientProxy() []string {
	var out []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "FTP_PROXY", "NO_PROXY", "ALL_PROXY"} {
		out = append(out, "-e", k+"=", "-e", strings.ToLower(k)+"=")
	}
	return out
}

// create makes a container of devvm's from an image: labelled, running
// Docker's init (tini) with a sleep under it, as root, sized, with the
// config's network, mounts and environment.
func (d *Docker) create(ctx context.Context, step msg.M, name, image string, labels map[string]string, c config, cpus int, memGiB float64) error {
	args := []string{"container", "create", "--name", name, "--label", LabelKey + "=" + d.Data}
	for _, k := range slices.Sorted(maps.Keys(labels)) {
		args = append(args, "--label", k+"="+labels[k])
	}
	args = append(args, "--init", "--user", "0:0", "--entrypoint", "sleep")
	args = append(args, noClientProxy()...)
	args = append(args, sizeArgs(cpus, memGiB)...)
	args = append(args, c.args()...)
	if err := d.Runner.Run(ctx, step, d.docker(ctx, append(args, image, "infinity")...)); err != nil {
		return backend.Logged(msg.VMCreateFailed(name))
	}
	return nil
}

// CreateVM makes a new, stopped container from the image, with its own
// home volume (the image's home copied in) and, for --clone, a volume for
// its clone; for the docker layer, under Sysbox (DockerLayer), with volumes for
// Docker's engine in it. Its CPUs and memory are base's unless spec sets
// them; its disk is the engine's.
func (d *Docker) CreateVM(ctx context.Context, img string, spec backend.VMSpec) error {
	vm := spec.Name
	if err := d.checkCreate(ctx, img, spec); err != nil {
		return err
	}
	cpus, mem, _, err := d.baseSize()
	if err != nil {
		return err
	}
	if spec.Size.CPUs > 0 {
		cpus = spec.Size.CPUs
	}
	if spec.Size.MemoryGiB > 0 {
		mem = spec.Size.MemoryGiB
	}
	r := record{
		Image: img, Project: spec.Project, CloneAt: spec.CloneAt, AgentDirs: slices.Clone(spec.AgentDirs),
		Tools: spec.Tools != "", Forwarding: spec.AgentForwarding, PortsAll: spec.Ports.All, Ports: slices.Clone(spec.Ports.Ranges),
	}
	if spec.DockerLayer {
		if _, err := d.DockerLayer(vm); err != nil {
			return err
		}
		r.Runtime = sysboxRuntime
	}
	c, err := d.vmConfig(vm, r)
	if err != nil {
		return err
	}
	for _, m := range append([]string{spec.Project}, spec.AgentDirs...) {
		if m == "" {
			continue
		}
		if err := d.checkShared(m); err != nil {
			return err
		}
	}
	// What a VM by this name deleted some other way left: its relay and
	// network, and its volumes.
	if err := (egress{d}).forget(ctx, vm); err != nil {
		return err
	}
	vols := []string{homeVolume(vm)}
	if spec.CloneAt != "" {
		vols = append(vols, cloneVolume(vm))
	}
	if r.dockerLayer() {
		vols = append(vols, engineVolumes(vm)...)
	}
	for _, v := range vols {
		if err := d.removeVolume(ctx, v); err != nil {
			return err
		}
		if err := d.createVolume(ctx, v); err != nil {
			return err
		}
	}
	// The tools volume: there, labelled (Docker would make one a container
	// names without), and up to the share.
	if r.Tools {
		if err := d.syncTools(ctx, d.imageRef(img)); err != nil {
			return err
		}
	}
	// Docker would make the network a container names unlabelled.
	if r.dockerLayer() {
		if err := (egress{d}).layerNetwork(ctx, vm); err != nil {
			return err
		}
	}
	if err := d.writeRecord(vm, r); err != nil {
		return err
	}
	return d.create(ctx, msg.Creating(vm), vm, d.imageRef(img), map[string]string{kindLabel: kindVM, configLabel: c.hash()}, c, cpus, mem)
}

// checkCreate checks what CreateVM is asked for: a valid image key, no
// disk size, the image built, and no container by the VM's name.
func (d *Docker) checkCreate(ctx context.Context, img string, spec backend.VMSpec) error {
	if !image.IsKey(img) {
		return fmt.Errorf("docker: invalid image key %q", img)
	}
	if spec.Size.DiskGiB > 0 {
		return msg.BackendLacks(Name, string(backend.FeatureDisk), spec.Name)
	}
	if _, err := d.inspectImage(ctx, d.imageRef(img)); err != nil {
		return msg.ImageMissing(img)
	}
	for _, n := range []string{spec.Name, nextName(spec.Name)} {
		if err := d.absent(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

// Start starts a stopped container, then what base's services do at boot
// on Lima (services.sh, Docker's daemon left out): containers run no init
// system. A running one is left as it is.
//
// What Docker fixes when it makes a container is compared first with what
// the VM wants now (its mounts, as config has them): where they differ,
// it's made again from a commit of its own (remake), with the same name
// and volumes. The shared tools' volume is brought up to the share first
// (syncTools). Then its network is set (egress.go): with NetProxy, its
// own network alone, with its relay to devvm's proxy, and once it runs,
// the proxy's address in its /etc/environment and the lifeline the proxy
// follows; else Docker's default one. With c.SSH, services.sh writes its
// sshd's config and host key, which is pinned (ssh.go); its ssh sessions
// forward the agent c chooses (agent.go); its forwarder starts once
// that's done, for the ports it forwards (ports.go).
func (d *Docker) Start(ctx context.Context, vm string, c backend.Confinement) error {
	if err := checkProxy(d.Caps(), vm, c); err != nil {
		return err
	}
	ct, err := d.vm(ctx, vm)
	if err != nil {
		return err
	}
	if ct.status() == backend.StatusRunning {
		return nil
	}
	r, err := d.readRecord(vm)
	if err != nil {
		return err
	}
	// Root in it can change its own network.
	if c.Net == backend.NetProxy && r.dockerLayer() {
		return msg.NetDockerLayer(Name, vm)
	}
	// Its own network and its engine's volumes, before a container made
	// again names them (a remake).
	if r.dockerLayer() {
		for _, v := range engineVolumes(vm) {
			if err := d.ensureVolume(ctx, v); err != nil {
				return err
			}
		}
		if err := (egress{d}).layerNetwork(ctx, vm); err != nil {
			return err
		}
	}
	if err := d.prepare(ctx, vm, ct, &r); err != nil {
		return err
	}
	e := egress{d}
	if err := e.vmNetwork(ctx, vm, &r, c); err != nil {
		return err
	}
	// Recorded before the start: one that fails leaves it without ssh, and
	// unconfined.
	r.SSHUp, r.NoSSHD, r.Started, r.Agent, r.Confined = false, false, "", sessionAgent(r, c), ""
	if err := d.writeRecord(vm, r); err != nil {
		return err
	}
	d.Forwards.Stop(vm)
	if err := os.Remove(d.Lifeline(ctx, vm)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := d.Runner.Run(ctx, msg.Starting(vm), d.docker(ctx, "container", "start", vm)); err != nil {
		return backend.Logged(msg.StartFailed(vm))
	}
	if err := e.setEnv(ctx, vm, proxyEnv(c)); err != nil {
		if ctx.Err() == nil {
			_ = d.stop(ctx, vm)
		}
		return err
	}
	sshd, err := d.sshdArgs(ctx, vm, &r, c)
	if err != nil {
		return err
	}
	if err := d.services(ctx, vm, r, sshd); err != nil {
		return err
	}
	if ct, err = d.vm(ctx, vm); err != nil {
		return err
	}
	key := ""
	if sshd != nil {
		if key, err = d.pinHostKey(ctx, vm); err != nil {
			return err
		}
	}
	r.Started, r.SSHUp = ct.State.StartedAt, sshd != nil
	// Through devvm's proxy: this start is the confined one, and the
	// proxy follows it.
	if c.Net == backend.NetProxy {
		r.Confined = ct.ID + " " + ct.State.StartedAt
		if err := e.follow(ctx, ct.ID, d.Lifeline(ctx, vm)); err != nil {
			return err
		}
	}
	if err := d.writeRecord(vm, r); err != nil {
		return err
	}
	d.forward(ctx, vm, ct, r, c, key)
	return nil
}

// checkProxy refuses a start through devvm's proxy without its port and
// password, or on an engine that can't reach it (caps).
func checkProxy(caps backend.Caps, vm string, c backend.Confinement) error {
	if c.Net != backend.NetProxy {
		return nil
	}
	if s := caps[backend.FeatureEgressProxy]; !s.IsEmulated() {
		return msg.BackendLacks(Name, string(backend.FeatureEgressProxy), vm)
	}
	if !backend.ValidPort(c.ProxyPort) || c.ProxyPassword == "" {
		return fmt.Errorf("docker: %s: no proxy to start it through", vm)
	}
	return nil
}

// prepare gets a stopped VM's container ready for a start with c: the
// shared tools' volume brought up to the share, and the container made
// again where what Docker fixed differs from what the VM wants.
func (d *Docker) prepare(ctx context.Context, vm string, ct container, r *record) error {
	want, err := d.vmConfig(vm, *r)
	if err != nil {
		return err
	}
	// First: Docker would make the volume unlabelled if it's gone.
	if r.Tools {
		if err := d.syncTools(ctx, d.imageRef(r.Image)); err != nil {
			return err
		}
	}
	if ct.Config.Labels[configLabel] == want.hash() {
		return nil
	}
	for _, m := range append([]string{r.Project}, r.AgentDirs...) {
		if m == "" {
			continue
		}
		if err := d.checkShared(m); err != nil {
			return err
		}
	}
	return d.remake(ctx, vm, ct, r, want)
}

// services runs services.sh in a running container, with args (its sshd's,
// ssh.go), without loop devices (its tools are folders, linked), its
// sshd's daemon (each connection runs its own, Connect), and Docker's
// daemon but in a VM made for the docker layer (dockerd can't run in
// another), then gives a --clone VM's user its clone's folder, a volume's
// root; a failure stops it.
func (d *Docker) services(ctx context.Context, vm string, r record, args []string) error {
	script, err := d.script("container/services.sh")
	if err != nil {
		return err
	}
	env := []string{"DEVVM_LOOP=off", "DEVVM_SSHD=inetd"}
	if !r.dockerLayer() {
		env = append([]string{"DEVVM_DOCKERD=off"}, env...)
	}
	cmd := d.asRoot(ctx, vm, env, args...)
	cmd.Stdin = bytes.NewReader(script)
	err = d.Runner.Run(ctx, msg.M{}, cmd)
	if err == nil && r.CloneAt != "" {
		name, _, _, uerr := d.user()
		if uerr != nil {
			return uerr
		}
		err = d.Runner.Run(ctx, msg.M{}, d.docker(ctx, "exec", "-u", "0", vm, "chown", name, r.CloneAt))
	}
	if err != nil {
		if ctx.Err() == nil {
			_ = d.stop(ctx, vm)
		}
		return backend.Logged(msg.StartFailed(vm))
	}
	return nil
}

// remake makes a stopped VM's container again with config c, keeping its
// disk: committed (as commitRef), or flattened when that has more than
// maxLayers layers, a new container made from that under a name of its
// own (nextName), the old one deleted, the new one renamed. Its CPUs,
// memory and volumes are kept. The record names the commit before the old
// container goes, and the commit before it is deleted last: a run stopped
// on the way leaves the disk in one or the other. Docker keeps the image's
// layers while the commits use them.
func (d *Docker) remake(ctx context.Context, vm string, old container, r *record, c config) error {
	d.Info(msg.DockerRemaking(vm))
	if err := d.removeIfThere(ctx, nextName(vm)); err != nil {
		return err
	}
	prev, ref := r.Commit, d.commitRef(vm, r.Gen+1)
	if err := d.commit(ctx, vm, ref, kindDisk, ""); err != nil {
		return err
	}
	img, err := d.inspectImage(ctx, ref)
	if err != nil {
		return err
	}
	if len(img.RootFS.Layers) > maxLayers {
		if err := d.flatten(ctx, vm, ref, img); err != nil {
			return err
		}
	}
	r.Commit, r.Gen = ref, r.Gen+1
	if err := d.writeRecord(vm, *r); err != nil {
		return err
	}
	sz := old.size()
	labels := map[string]string{kindLabel: kindVM, configLabel: c.hash()}
	if err := d.create(ctx, msg.Creating(vm), nextName(vm), ref, labels, c, sz.CPUs, sz.MemoryGiB); err != nil {
		return err
	}
	if err := d.removeContainer(ctx, msg.Configuring(vm), vm); err != nil {
		return err
	}
	if err := d.Runner.Run(ctx, msg.Configuring(vm), d.docker(ctx, "container", "rename", nextName(vm), vm)); err != nil {
		return backend.Logged(msg.DockerUpdateFailed(vm))
	}
	if prev != "" && prev != ref {
		return d.removeImage(ctx, prev)
	}
	return nil
}

// flatten makes ref a single layer: the stopped container's files as they
// are (docker export), imported with the image's environment and devvm's
// labels. Its layers are no longer shared with its image's.
func (d *Docker) flatten(ctx context.Context, vm, ref string, img dockerImage) error {
	changes := []string{"LABEL " + LabelKey + "=" + strconv.Quote(d.Data), "LABEL " + kindLabel + "=" + kindDisk}
	for _, e := range img.Config.Env {
		k, v, ok := strings.Cut(e, "=")
		if !ok || strings.ContainsAny(k, " \t\n\"\\") {
			continue
		}
		changes = append(changes, "ENV "+k+"="+strconv.Quote(v))
	}
	export := d.docker(ctx, "container", "export", vm)
	out, err := export.StdoutPipe()
	if err != nil {
		return err
	}
	args := []string{"image", "import"}
	for _, ch := range changes {
		args = append(args, "--change", ch)
	}
	imp := d.docker(ctx, append(args, "-", ref)...)
	imp.Stdin = out
	if err := export.Start(); err != nil {
		return backend.Logged(msg.DockerCommitFailed(vm))
	}
	err = d.Runner.Run(ctx, msg.CommittingImage(vm), imp)
	_, _ = io.Copy(io.Discard, out)
	if werr := export.Wait(); err == nil {
		err = werr
	}
	if err != nil {
		return backend.Logged(msg.DockerCommitFailed(vm))
	}
	return nil
}
