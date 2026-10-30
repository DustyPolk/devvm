package smolvm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// dockerDir is a folder on a machine's own disk, not its overlay root,
// which Docker's storage needs.
const dockerDir = "/var/lib/docker"

// BuildImage builds an image's pack. Base is a machine made from Ubuntu's
// pinned container image (or spec.From's), by its digest, prepared as Lima's cloud
// image comes (prepare.sh: sudo, CA certificates, the VM's user; after its
// scripts, sshd.sh: sshd, without host keys); a layer
// a machine made from its parent's pack. Neither mounts anything from the
// host. It starts with the network spec.Net asks for, runs the scripts in
// order as the VM's user (while shims.sh's stand-ins for systemctl and
// rc-update are there), stops, and is packed into the image's folder; the
// machine is deleted then.
//
// Until the machine exists, an interrupted run deletes it, and so does a
// failed create: a cleanup on the ui stack. Once it exists, a failure keeps
// it, stopped, for inspection; the next build deletes it (it has no
// record).
func (s *Smolvm) BuildImage(ctx context.Context, spec backend.ImageSpec) (backend.Built, error) {
	if spec.From != nil && spec.Parent != "" {
		return backend.Built{}, errors.New("smolvm: only base starts from a container image")
	}
	if spec.Net != backend.NetInternet && spec.Net != backend.NetNone {
		return backend.Built{}, fmt.Errorf("smolvm: builds are confined, not started with network %d", spec.Net)
	}
	for _, k := range []string{spec.Key, spec.Parent} {
		if k != "" && !keyPattern.MatchString(k) {
			return backend.Built{}, fmt.Errorf("smolvm: invalid image key %q", k)
		}
	}
	inst := ImagePrefix + spec.Key
	if err := s.absent(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	if _, err := os.Stat(s.Dirs.SmolvmImage(spec.Key)); err == nil {
		return backend.Built{}, fmt.Errorf("smolvm: image %s exists already", spec.Key)
	}
	if spec.Parent != "" {
		if _, err := os.Stat(s.packFile(spec.Parent)); err != nil {
			return backend.Built{}, msg.ImageMissing(spec.Parent)
		}
	}
	name, uid, err := s.user()
	if err != nil {
		return backend.Built{}, err
	}
	cpus, mem, disk, err := s.baseSize()
	if err != nil {
		return backend.Built{}, err
	}
	if spec.MemoryGiB > 0 {
		mem = spec.MemoryGiB
	}

	var created atomic.Bool // an interrupt's cleanups may run while this goes on
	remove := s.AddCleanup(func(ctx context.Context) {
		if !created.Load() || s.Interrupted() {
			s.unfinished(ctx, inst)
		}
	})
	// Its steps: create, start, the stand-ins, each script, the stand-ins
	// gone, stop, pack, delete; base's preparation first and its sshd
	// after its scripts.
	steps := 7 + len(spec.Scripts)
	if spec.Parent == "" {
		steps += 2
	}
	ctx = s.WithSteps(ctx, steps)
	built := backend.Built{}
	args := []string{
		"machine", "create", "--name", inst, "--label", LabelKey + "=" + s.Dirs.Data,
		"--cpus", strconv.Itoa(cpus), "--mem", mib(mem), "--storage", gibCeil(disk), "-v", dockerDir,
	}
	if spec.Net == backend.NetInternet {
		// Not set later (Start's update): a pack carries its network.
		args = append(args, "--net")
	}
	step := msg.CloningImage(image.Parent(spec.Image))
	if spec.Parent == "" {
		ref, origin, err := s.baseImage(spec.From)
		if err != nil {
			return backend.Built{}, err
		}
		built.Origin = origin
		args = append(args, "--image", ref)
		step = msg.CreatingFromImage(ref, inst)
	} else {
		args = append(args, "--from", s.packFile(spec.Parent))
	}
	if err := s.Runner.Run(ctx, step, s.smolvm(ctx, args...)); err != nil {
		return backend.Built{}, backend.Logged(msg.VMCreateFailed(inst))
	}
	if err := s.writeRecord(inst, record{Image: spec.Parent}); err != nil {
		return backend.Built{}, err
	}
	created.Store(true)
	// A failed start leaves it stopped.
	if err := s.Start(ctx, inst, backend.Confinement{Net: spec.Net}); err != nil {
		return backend.Built{}, err
	}
	// Once it's running, a failure stops it (unless the run was
	// interrupted: then it's deleted).
	failed := func(err error) (backend.Built, error) {
		if ctx.Err() == nil {
			_ = s.stop(ctx, inst)
		}
		return backend.Built{}, err
	}
	if spec.Parent == "" {
		if err := s.rootScript(ctx, inst, "prepare.sh", name, uid); err != nil {
			return failed(err)
		}
		out, err := s.Runner.Output(s.smolvm(ctx, "machine", "exec", "--name", inst, "--",
			"sh", "-c", "if command -v apk >/dev/null; then echo musl; else echo gnu; fi"))
		if err != nil {
			return failed(backend.Logged(msg.SmolvmScriptFailed("prepare.sh", inst)))
		}
		built.Libc = strings.TrimSpace(string(out))
	}
	if err := s.rootScript(ctx, inst, "shims.sh", "on"); err != nil {
		return failed(err)
	}
	for _, sc := range spec.Scripts {
		if err := s.runScript(ctx, inst, sc); err != nil {
			return failed(err)
		}
	}
	if spec.Parent == "" {
		if err := s.rootScript(ctx, inst, "sshd.sh"); err != nil {
			return failed(err)
		}
	}
	if err := s.rootScript(ctx, inst, "shims.sh", "off"); err != nil {
		return failed(err)
	}
	if err := s.stop(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	if err := s.packInto(ctx, inst, spec.Key); err != nil {
		return backend.Built{}, err
	}
	if err := s.remove(ctx, inst); err != nil {
		return backend.Built{}, err
	}
	remove()
	return built, nil
}

// baseImage is the container image base starts from, by digest, and what
// its record says it started from: spec's (as recorded), else Ubuntu's,
// pinned. One only Docker has is refused: smolvm pulls from registries.
func (s *Smolvm) baseImage(f *backend.From) (ref, origin string, err error) {
	if f != nil {
		if f.Digest == "" {
			return "", "", msg.NoDigest(f.Ref)
		}
		if !from.Valid(f.Ref) || !from.Valid(f.Digest) {
			return "", "", msg.BadFrom(msg.Shown(f.Ref))
		}
		if !strings.Contains(f.Digest, "@") {
			return "", "", msg.SmolvmFromRegistryOnly(msg.Shown(f.Ref))
		}
		return f.Digest, f.Digest, nil
	}
	return ubuntu.ContainerRef, ubuntu.ContainerOrigin, nil
}

// rootScript runs one of devvm's scripts for images run as containers
// (containerScript) as root in a running machine, fed to sh with args.
func (s *Smolvm) rootScript(ctx context.Context, vm, name string, args ...string) error {
	script, err := s.containerScript(name)
	if err != nil {
		return err
	}
	cmd := s.asRoot(ctx, vm, args...)
	cmd.Stdin = bytes.NewReader(script)
	if err := s.Runner.Run(ctx, msg.RunningScript(name, vm), cmd); err != nil {
		return backend.Logged(msg.SmolvmScriptFailed(name, vm))
	}
	return nil
}

// runScript runs a script in the machine as its user, fed to bash.
func (s *Smolvm) runScript(ctx context.Context, vm string, sc backend.Script) error {
	args, err := s.userArgs(vm, "", true, false, []string{"bash", "-s"})
	if err != nil {
		return err
	}
	cmd := s.smolvm(ctx, args...)
	cmd.Stdin = bytes.NewReader(sc.Data)
	if err := s.Runner.Run(ctx, msg.RunningScript(sc.Name, vm), cmd); err != nil {
		return backend.Logged(msg.SmolvmScriptFailed(sc.Name, vm))
	}
	return nil
}

// packInto packs a stopped machine into the image's folder: written beside
// it, then renamed in, so a folder that's there is a whole pack. smolvm's
// stub is kept with it: machines made from the pack read it when they grow.
func (s *Smolvm) packInto(ctx context.Context, vm, key string) error {
	dir := s.Dirs.SmolvmImage(key)
	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return err
	}
	done := s.AddCleanup(func(context.Context) { _ = os.RemoveAll(tmp) })
	defer done()
	if err := s.Runner.Run(ctx, msg.PackingImage(vm),
		s.smolvm(ctx, "pack", "create", "--from-vm", vm, "-o", filepath.Join(tmp, "image"))); err != nil {
		_ = os.RemoveAll(tmp)
		return backend.Logged(msg.PackFailed(vm))
	}
	if _, err := os.Stat(filepath.Join(tmp, "image.smolmachine")); err != nil {
		_ = os.RemoveAll(tmp)
		return backend.Logged(msg.PackFailed(vm))
	}
	return os.Rename(tmp, dir)
}

// unfinished deletes an image's build machine that didn't finish: on an
// interrupt, or when its create failed.
func (s *Smolvm) unfinished(ctx context.Context, inst string) {
	if _, err := s.inspect(ctx, inst); err != nil {
		return // never created, someone else's, or smolvm can't say
	}
	if s.Interrupted() {
		s.Info(msg.InterruptedImage(inst))
	} else {
		s.Info(msg.UnfinishedImage(inst))
	}
	_ = s.remove(ctx, inst)
}

// Images lists the keys of devvm's images on smolvm: its packs, and the
// build machines of those that didn't finish.
func (s *Smolvm) Images(ctx context.Context) ([]string, error) {
	var keys []string
	entries, err := os.ReadDir(filepath.Join(s.Dirs.Smolvm(), "images"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if keyPattern.MatchString(e.Name()) && e.IsDir() {
			keys = append(keys, e.Name())
		}
	}
	ms, err := s.ours(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if k, ok := imageKey(m.Name); ok && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// ImageSize is the disk space of an image's pack, and of its build
// machine while it has one.
func (s *Smolvm) ImageSize(ctx context.Context, key string) (int64, error) {
	if !keyPattern.MatchString(key) {
		return 0, msg.NotAnImage(ImagePrefix + key)
	}
	var total int64
	found := false
	if n, err := platform.DiskUsage(s.Dirs.SmolvmImage(key)); err == nil {
		total, found = n, true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return 0, err
	}
	if _, err := s.inspect(ctx, ImagePrefix+key); err == nil {
		if dir := s.dataDir(ctx, ImagePrefix+key); dir != "" {
			if n, err := platform.DiskUsage(dir); err == nil {
				total, found = total+n, true
			}
		}
	}
	if !found {
		return 0, fmt.Errorf("image %s: %w", key, fs.ErrNotExist)
	}
	return total, nil
}

// dataDir is smolvm's folder for a machine, "" when it can't say.
func (s *Smolvm) dataDir(ctx context.Context, vm string) string {
	out, err := s.Runner.Output(s.smolvm(ctx, "machine", "data-dir", "--name", vm))
	dir := strings.TrimSpace(string(out))
	if err != nil || !filepath.IsAbs(dir) {
		return ""
	}
	return dir
}

// DeleteImage deletes an image's pack and its build machine. One that VMs
// were made from is refused: they read its pack when their disks grow.
func (s *Smolvm) DeleteImage(ctx context.Context, key string) error {
	if !keyPattern.MatchString(key) {
		return msg.NotAnImage(ImagePrefix + key)
	}
	users, err := s.imageUsers(ctx, key)
	if err != nil {
		return err
	}
	if len(users) > 0 {
		return msg.ImageInUse(key, strings.Join(users, " "))
	}
	if err := s.removeIfThere(ctx, ImagePrefix+key); err != nil {
		return err
	}
	return os.RemoveAll(s.Dirs.SmolvmImage(key))
}

// imageUsers are devvm's machines made from the image's pack.
func (s *Smolvm) imageUsers(ctx context.Context, key string) ([]string, error) {
	ms, err := s.ours(ctx)
	if err != nil {
		return nil, err
	}
	var users []string
	for _, m := range ms {
		if m.Name == ImagePrefix+key {
			continue
		}
		r, err := s.readRecord(m.Name)
		if err != nil {
			return nil, err
		}
		if r.Image == key {
			users = append(users, m.Name)
		}
	}
	return users, nil
}

// CreateVM makes a new, stopped machine from the image's pack, with
// everything set: a pack would give it its build machine's network, CPUs
// and memory. Its disk is at least base's. Its ports are its forwarder's
// (forward.go), so smolvm forwards none of them.
func (s *Smolvm) CreateVM(ctx context.Context, img string, spec backend.VMSpec) error {
	vm := spec.Name
	if !keyPattern.MatchString(img) {
		return fmt.Errorf("smolvm: invalid image key %q", img)
	}
	if _, err := os.Stat(s.packFile(img)); err != nil {
		return msg.ImageMissing(img)
	}
	if err := s.absent(ctx, vm); err != nil {
		return err
	}
	cpus, mem, disk, err := s.baseSize()
	if err != nil {
		return err
	}
	if spec.Size.CPUs > 0 {
		cpus = spec.Size.CPUs
	}
	if spec.Size.MemoryGiB > 0 {
		mem = spec.Size.MemoryGiB
	}
	disk = max(disk, spec.Size.DiskGiB)
	args := []string{
		"machine", "create", "--name", vm, "--label", LabelKey + "=" + s.Dirs.Data, "--from", s.packFile(img),
		"--cpus", strconv.Itoa(cpus), "--mem", mib(mem), "--storage", gibCeil(disk), "-v", dockerDir,
	}
	var vols []string
	if spec.Project != "" {
		v, err := volume(spec.Project, spec.Project, true)
		if err != nil {
			return err
		}
		vols = append(vols, v)
	}
	for _, d := range spec.AgentDirs {
		v, err := volume(d, d, false)
		if err != nil {
			return err
		}
		vols = append(vols, v)
	}
	if spec.Tools != "" {
		v, err := volume(spec.Tools, spec.ToolsAt, false)
		if err != nil {
			return err
		}
		vols = append(vols, v)
	}
	for _, v := range vols {
		args = append(args, "-v", v)
	}
	// Forwarding is fixed now: a VM whose forwarding may change later
	// forwards from the start, given no agent while it's off (agent.go).
	forwarding := spec.AgentForwarding || spec.AgentToggle
	if forwarding {
		args = append(args, "--ssh-agent")
	}
	if err := s.Runner.Run(ctx, msg.Creating(vm), s.smolvm(ctx, args...)); err != nil {
		return backend.Logged(msg.VMCreateFailed(vm))
	}
	return s.writeRecord(vm, record{
		Image: img, Project: spec.Project, AgentDirs: slices.Clone(spec.AgentDirs), Tools: spec.Tools, ToolsAt: spec.ToolsAt,
		Forwarding: forwarding, AgentOff: forwarding && !spec.AgentForwarding,
		PortsAll: spec.Ports.All, Listed: listed(spec.Ports),
	})
}

var (
	cpusLine   = regexp.MustCompile(`(?m)^cpus:\s*([0-9]+)\s*$`)
	memoryLine = regexp.MustCompile(`(?m)^memory:\s*([0-9.]+)GiB\s*$`)
	diskLine   = regexp.MustCompile(`(?m)^disk:\s*([0-9.]+)GiB\s*$`)
)

// gib is a GiB in bytes.
const gib = 1 << 30

// baseSize is what images and VMs get unless told otherwise: base.yaml's
// CPUs, memory and disk, as Lima's base has them; the memory halved when
// it would leave the host less than 2 GiB.
func (s *Smolvm) baseSize() (cpus int, mem, disk float64, err error) {
	data, err := fs.ReadFile(s.Assets, "base.yaml")
	if err != nil {
		return 0, 0, 0, err
	}
	get := func(re *regexp.Regexp) (float64, error) {
		m := re.FindSubmatch(data)
		if m == nil {
			return 0, fmt.Errorf("smolvm: base.yaml doesn't match %s", re)
		}
		return strconv.ParseFloat(string(m[1]), 64)
	}
	c, err := get(cpusLine)
	if err != nil {
		return 0, 0, 0, err
	}
	if mem, err = get(memoryLine); err != nil {
		return 0, 0, 0, err
	}
	if disk, err = get(diskLine); err != nil {
		return 0, 0, 0, err
	}
	host, err := s.Memory()
	if err != nil {
		return 0, 0, 0, err
	}
	if h := float64(host / gib); mem+2 > h {
		mem = h / 2
	}
	return int(c), mem, disk, nil
}
