package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// BuildImage builds an image: a container made from its parent's image,
// or for base from Ubuntu's pinned container image (or spec.From's, which Docker
// has or pulls), by its digest, prepared as Lima's cloud image comes
// (prepare.sh: sudo, CA certificates, the VM's user; after its scripts,
// sshd.sh: sshd, without host keys). It mounts nothing from the host and
// gets the network spec.Net asks for; it runs the scripts in order as the
// VM's user (while shims.sh's stand-ins for systemctl and rc-update are
// there), stops, and is committed as the image (imageRef); the container
// is deleted then.
//
// Until the container exists, an interrupted run deletes it, and so does
// a failed create: a cleanup on the ui stack. Once it exists, a failure
// keeps it, stopped, for inspection; devvm gc deletes it.
func (d *Docker) BuildImage(ctx context.Context, spec backend.ImageSpec) (backend.Built, error) {
	if err := d.checkBuild(ctx, spec); err != nil {
		return backend.Built{}, err
	}
	name, uid, gid, err := d.user()
	if err != nil {
		return backend.Built{}, err
	}
	cpus, mem, _, err := d.baseSize()
	if err != nil {
		return backend.Built{}, err
	}
	if spec.MemoryGiB > 0 {
		mem = spec.MemoryGiB
	}
	inst := d.buildName(spec.Key)
	var created atomic.Bool // an interrupt's cleanups may run while this goes on
	remove := d.AddCleanup(func(ctx context.Context) {
		if !created.Load() || d.Interrupted() {
			d.unfinished(ctx, inst)
		}
	})
	// Its steps: create, start, the stand-ins, each script, the stand-ins
	// gone, stop, commit, delete; base's pull and preparation first, and
	// its sshd after its scripts.
	steps := 7 + len(spec.Scripts)
	if spec.Parent == "" {
		steps += 3
	}
	if !spec.Trust.None() {
		steps += 2 // the user's CAs added before the rest, and removed
	}
	ctx = d.WithSteps(ctx, steps)
	built := backend.Built{}
	ref, step := d.imageRef(spec.Parent), msg.CloningImage(image.Parent(spec.Image))
	if spec.Parent == "" {
		var origin string
		if ref, origin, err = d.baseImage(ctx, spec.From); err != nil {
			return backend.Built{}, err
		}
		built.Origin, step = origin, msg.CreatingFromImage(ref, inst)
		if built.Fitted, err = fitted(d.Assets, cpus, mem); err != nil {
			return backend.Built{}, err
		}
	}
	labels := map[string]string{kindLabel: kindBuild, keyLabel: spec.Key}
	ctx, netw, lifeline, done, err := egress{d}.buildNet(ctx, inst, spec.Net)
	if err != nil {
		return backend.Built{}, err
	}
	undo := d.AddCleanup(done)
	c := config{Hostname: "devvm-build", Net: netw, CapDrop: buildCaps(inst, netw)}
	if err := d.create(ctx, step, inst, ref, labels, c, cpus, mem); err != nil {
		return backend.Built{}, err
	}
	created.Store(true)
	if err := (egress{d}).startBuild(ctx, inst, lifeline); err != nil {
		return backend.Built{}, err
	}
	if built.Libc, err = d.runBuild(ctx, inst, spec, name, uid, gid); err != nil {
		if ctx.Err() == nil {
			_ = d.stop(ctx, inst)
		}
		return backend.Built{}, err
	}
	if err := d.stop(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	if err := d.commit(ctx, inst, d.imageRef(spec.Key), kindImage, spec.Key); err != nil {
		return backend.Built{}, err
	}
	if err := d.removeContainer(ctx, msg.Deleting(inst), inst); err != nil {
		return backend.Built{}, err
	}
	remove()
	undo()
	done(ctx)
	return built, nil
}

// startBuild starts a build's container (inst), and with lifeline the
// lifeline its proxy follows (egress.go).
func (e egress) startBuild(ctx context.Context, inst, lifeline string) error {
	if err := e.d.Runner.Run(ctx, msg.Starting(inst), e.d.docker(ctx, "container", "start", inst)); err != nil {
		return backend.Logged(msg.StartFailed(inst))
	}
	if lifeline == "" {
		return nil
	}
	c, err := e.d.inspect(ctx, inst)
	if err != nil {
		return err
	}
	return e.follow(ctx, c.ID, lifeline)
}

// checkBuild checks what BuildImage is asked for: the network, the keys,
// the parent built and the image not.
func (d *Docker) checkBuild(ctx context.Context, spec backend.ImageSpec) error {
	if spec.From != nil && spec.Parent != "" {
		return errors.New("docker: only base starts from a container image")
	}
	if err := checkBuildNet(d.Caps(), spec.Image, spec.Net); err != nil {
		return err
	}
	for _, k := range []string{spec.Key, spec.Parent} {
		if k != "" && !keyPattern.MatchString(k) {
			return fmt.Errorf("docker: invalid image key %q", k)
		}
	}
	if err := d.absent(ctx, d.buildName(spec.Key)); err != nil {
		return err
	}
	if _, err := d.inspectImage(ctx, d.imageRef(spec.Key)); err == nil {
		return fmt.Errorf("docker: image %s exists already", spec.Key)
	} else if !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	if spec.Parent != "" {
		if _, err := d.inspectImage(ctx, d.imageRef(spec.Parent)); err != nil {
			return msg.ImageMissing(spec.Parent)
		}
	}
	return nil
}

// checkBuildNet refuses a build's network that isn't a confined one, and
// a build (name) on an engine where devvm can't confine it (caps): its
// caller checked an emulation's review.
func checkBuildNet(caps backend.Caps, name string, n backend.Net) error {
	switch n {
	case backend.NetInternet, backend.NetNone:
	case backend.NetDirect, backend.NetProxy:
		return fmt.Errorf("docker: builds are confined, not started with network %d", n)
	}
	if s := caps[backend.FeatureBuildConfinement]; !s.IsNative() && !s.IsEmulated() {
		return msg.BackendLacks(Name, string(backend.FeatureBuildConfinement), name)
	}
	return nil
}

// runBuild runs a build's scripts in its running container, and returns
// base's C library ("" above base).
func (d *Docker) runBuild(ctx context.Context, inst string, spec backend.ImageSpec, name, uid, gid string) (string, error) {
	libc := ""
	if err := d.runScript(ctx, inst, spec.Trust.Add, true); err != nil {
		return "", err
	}
	if spec.Parent == "" {
		args := []string{name, uid}
		if d.OS == "linux" {
			args = append(args, gid) // the host's bind mounts keep it
		}
		if err := d.rootScript(ctx, inst, "prepare.sh", args...); err != nil {
			return "", err
		}
		out, err := d.Runner.Output(d.docker(ctx, "exec", inst,
			"sh", "-c", "if command -v apk >/dev/null; then echo musl; else echo gnu; fi"))
		if err != nil {
			return "", backend.Logged(msg.DockerScriptFailed("prepare.sh", inst))
		}
		libc = strings.TrimSpace(string(out))
	}
	if err := d.rootScript(ctx, inst, "shims.sh", "on"); err != nil {
		return "", err
	}
	e := egress{d}
	if err := e.buildSudo(ctx, inst, true); err != nil {
		return "", err
	}
	for _, sc := range spec.Scripts {
		if err := d.runScript(ctx, inst, sc, false); err != nil {
			return "", err
		}
	}
	if spec.Parent == "" {
		if err := d.rootScript(ctx, inst, "sshd.sh"); err != nil {
			return "", err
		}
	}
	if err := d.runScript(ctx, inst, spec.Trust.Remove, true); err != nil {
		return "", err
	}
	if err := e.buildSudo(ctx, inst, false); err != nil {
		return "", err
	}
	return libc, d.rootScript(ctx, inst, "shims.sh", "off")
}

// baseImage is the container image base starts from, pulled when it's a
// registry's, and what its record says it started from: spec's (as
// recorded: a registry digest, or Docker's own ID for an image only Docker
// has), else Ubuntu's, pinned.
func (d *Docker) baseImage(ctx context.Context, f *backend.From) (ref, origin string, err error) {
	if f != nil {
		if f.Digest == "" {
			return "", "", msg.NoDigest(f.Ref)
		}
		if !from.Valid(f.Ref) || !from.Valid(f.Digest) {
			return "", "", msg.BadFrom(f.Ref)
		}
		if !strings.Contains(f.Digest, "@") {
			// Docker's ID: only Docker has it, and only while it's there.
			if _, err := d.Runner.Output(d.docker(ctx, "image", "inspect", "--format", "{{.Id}}", f.Digest)); err != nil {
				return "", "", msg.DockerImageChanged(f.Ref)
			}
			return f.Digest, f.Digest, nil
		}
		return f.Digest, f.Digest, d.pull(ctx, f.Digest)
	}
	return ubuntu.ContainerRef, ubuntu.ContainerOrigin, d.pull(ctx, ubuntu.ContainerRef)
}

// rootScript runs one of the scripts devvm runs in images as containers
// (container/<name>) as root in a running container, fed to sh with args.
func (d *Docker) rootScript(ctx context.Context, name, script string, args ...string) error {
	data, err := d.script("container/" + script)
	if err != nil {
		return err
	}
	cmd := d.asRoot(ctx, name, nil, args...)
	cmd.Stdin = bytes.NewReader(data)
	if err := d.Runner.Run(ctx, msg.RunningScript(script, name), cmd); err != nil {
		return backend.Logged(msg.DockerScriptFailed(script, name))
	}
	return nil
}

// runScript runs a script in the container as the VM's user, fed to bash,
// or with root as root, fed to sh: a build's Trust scripts, of which an
// empty one isn't run.
func (d *Docker) runScript(ctx context.Context, name string, sc backend.Script, root bool) error {
	var cmd *exec.Cmd
	if root {
		if len(sc.Data) == 0 {
			return nil
		}
		cmd = d.asRoot(ctx, name, nil)
	} else {
		args, err := d.userArgs(ctx, name, "", true, false, []string{"bash", "-s"})
		if err != nil {
			return err
		}
		cmd = d.docker(ctx, args...)
	}
	cmd.Stdin = bytes.NewReader(sc.Data)
	if err := d.Runner.Run(ctx, msg.RunningScript(sc.Name, name), cmd); err != nil {
		return backend.Logged(msg.DockerScriptFailed(sc.Name, name))
	}
	return nil
}

// unfinished deletes an image's build container that didn't finish: on
// an interrupt, or when its create failed.
func (d *Docker) unfinished(ctx context.Context, inst string) {
	if _, err := d.inspect(ctx, inst); err != nil {
		return // never created, someone else's, or Docker can't say
	}
	if d.Interrupted() {
		d.Info(msg.InterruptedImage(inst))
	} else {
		d.Info(msg.UnfinishedImage(inst))
	}
	_ = d.removeContainer(ctx, msg.Deleting(inst), inst)
}

// Images lists the keys of devvm's images on Docker: its images, and the
// build containers of those that didn't finish.
func (d *Docker) Images(ctx context.Context) ([]string, error) {
	out, err := d.Runner.Output(d.docker(ctx, "image", "ls", "--filter", "label="+LabelKey+"="+d.Data,
		"--filter", "label="+kindLabel+"="+kindImage, "--format", "{{.Repository}}:{{.Tag}}"))
	if err != nil {
		return nil, fmt.Errorf("docker image ls: %w", err)
	}
	var keys []string
	for l := range strings.FieldsSeq(string(out)) {
		if k, ok := strings.CutPrefix(l, "devvm-"+d.ns()+":"); ok && keyPattern.MatchString(k) && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	cs, err := d.containers(ctx, kindBuild)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		if k := c.Config.Labels[keyLabel]; keyPattern.MatchString(k) && c.name() == d.buildName(k) && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// ImageSize is the disk space of an image, its parents' layers included,
// and of its build container's own while it has one.
func (d *Docker) ImageSize(ctx context.Context, key string) (int64, error) {
	if !keyPattern.MatchString(key) {
		return 0, msg.NotAnImage(key)
	}
	var total int64
	found := false
	if img, err := d.inspectImage(ctx, d.imageRef(key)); err == nil {
		total, found = img.Size, true
	} else if !errors.Is(err, backend.ErrNotFound) {
		return 0, err
	}
	if out, err := d.Runner.Output(d.docker(ctx, "container", "inspect", "--size", "--format", "{{.SizeRw}}", d.buildName(key))); err == nil {
		if n, perr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); perr == nil {
			total, found = total+n, true
		}
	}
	if !found {
		return 0, fmt.Errorf("image %s: %w", key, fs.ErrNotExist)
	}
	return total, nil
}

// DeleteImage deletes an image and its build container. One that VMs were
// made from is refused.
func (d *Docker) DeleteImage(ctx context.Context, key string) error {
	if !keyPattern.MatchString(key) {
		return msg.NotAnImage(key)
	}
	users, err := d.imageUsers(ctx, key)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return msg.ImageInUse(key, strings.Join(users, " "))
	}
	if err := d.removeIfThere(ctx, d.buildName(key)); err != nil {
		return err
	}
	return d.removeImage(ctx, d.imageRef(key))
}

// imageUsers are devvm's VMs made from the image.
func (d *Docker) imageUsers(ctx context.Context, key string) ([]string, error) {
	cs, err := d.containers(ctx, kindVM)
	if err != nil {
		return nil, err
	}
	var users []string
	for _, c := range cs {
		r, err := d.readRecord(c.name())
		if err != nil {
			return nil, err
		}
		if r.Image == key && !slices.Contains(users, c.name()) {
			users = append(users, c.name())
		}
	}
	return users, nil
}

var (
	cpusLine   = regexp.MustCompile(`(?m)^cpus:\s*([0-9]+)\s*$`)
	memoryLine = regexp.MustCompile(`(?m)^memory:\s*([0-9.]+)GiB\s*$`)
	diskLine   = regexp.MustCompile(`(?m)^disk:\s*([0-9.]+)GiB\s*$`)
)

// baseSize is what images and VMs get unless told otherwise: base.yaml's
// CPUs, memory and disk, as Lima's base has them, its CPUs and memory
// fitted to the engine's machine (backend.FitCPUs, backend.FitMemory) once
// Problems has read them.
func (d *Docker) baseSize() (cpus int, mem, disk float64, err error) {
	want, err := baseYAML(d.Assets)
	if err != nil {
		return 0, 0, 0, err
	}
	mem = want.MemoryGiB
	hostCPUs, hostMem := d.engineSize()
	if hostMem > 0 {
		mem = backend.FitMemory(mem, uint64(hostMem))
	}
	return backend.FitCPUs(want.CPUs, hostCPUs), mem, want.DiskGiB, nil
}

// baseYAML is base.yaml's CPUs, memory and disk in assets, not fitted to
// the engine's machine.
func baseYAML(assets fs.FS) (backend.Size, error) {
	data, err := fs.ReadFile(assets, "base.yaml")
	if err != nil {
		return backend.Size{}, err
	}
	get := func(re *regexp.Regexp) (float64, error) {
		m := re.FindSubmatch(data)
		if m == nil {
			return 0, fmt.Errorf("docker: base.yaml doesn't match %s", re)
		}
		return strconv.ParseFloat(string(m[1]), 64)
	}
	var want backend.Size
	c, err := get(cpusLine)
	if err != nil {
		return backend.Size{}, err
	}
	want.CPUs = int(c)
	if want.MemoryGiB, err = get(memoryLine); err != nil {
		return backend.Size{}, err
	}
	if want.DiskGiB, err = get(diskLine); err != nil {
		return backend.Size{}, err
	}
	return want, nil
}

// fitted is what base's build reports as fitted, for the CPUs and
// memory it gets (backend.Fitted) and assets' base.yaml.
func fitted(assets fs.FS, cpus int, mem float64) (backend.Size, error) {
	want, err := baseYAML(assets)
	return backend.Fitted(want, backend.Size{CPUs: cpus, MemoryGiB: mem}), err
}

// sizeArgs are docker's options that set CPUs and memory (without swap);
// none for zero.
func sizeArgs(cpus int, memGiB float64) []string {
	var out []string
	if cpus > 0 {
		out = append(out, "--cpus", strconv.Itoa(cpus))
	}
	if memGiB > 0 {
		m := strconv.Itoa(int(memGiB*1024)) + "m"
		out = append(out, "--memory", m, "--memory-swap", m)
	}
	return out
}
