// Package lima is the Lima backend: VMs run by limactl, in a Lima home of
// devvm's own. On macOS VMs run on Apple's Virtualization framework and are
// confined with Seatbelt (sandbox-exec); on Linux they run on QEMU and are
// confined with QEMU's restrict=on, through a wrapper (devvm _qemu).
package lima

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// Name is the backend's name.
const Name = "lima"

// ImagePrefix starts the name of every image's instance: devvm-img-<key>.
const ImagePrefix = "devvm-img-"

// sshTimeout is how long a VM that Lima says is running, after a start
// Lima reports as failed, gets to answer over ssh.
const sshTimeout = 60 * time.Second

// Lima is the backend. New sets every field; tests change them.
type Lima struct {
	Dirs   paths.Dirs
	Runner Runner
	// OS and Arch are the host's, as Go names them: how VMs are confined
	// (Seatbelt on darwin, QEMU on linux) and which QEMU Lima runs.
	OS, Arch string
	// Self is devvm's binary, which the QEMU wrapper and a build's proxy
	// run.
	Self string
	// Spawn starts a build's proxy, a process that can outlive devvm,
	// and returns it (platform.StartDetachedProcess); Tick is how often
	// it's looked at while it starts, ten times at most.
	Spawn func(path string, args []string, out *os.File) (*os.Process, error)
	Tick  time.Duration
	// Assets holds devvm's own files (assets.FS): base.yaml, oci/.
	Assets fs.FS
	// SSHTimeout is how long a degraded start waits for ssh.
	SSHTimeout time.Duration
	// OCIWait is how long a --from conversion waits between looks at
	// another's lock.
	OCIWait time.Duration
	// Getenv reads the environment (SSH_AUTH_SOCK).
	Getenv func(string) string
	// Ubuntu finds Ubuntu's cloud image for base.
	Ubuntu func(ctx context.Context) (ubuntu.Image, error)
	// Memory returns the host's memory in bytes.
	Memory func() (uint64, error)
	// CPUs returns the host's CPU count.
	CPUs func() int
	// MacOS returns macOS's version, "" when unknown.
	MacOS func() string
	// KVM is Linux's KVM device: /dev/kvm, unless DEVVM_KVM is set.
	KVM string
	// DockerRunning reports whether Docker runs, for --from images only
	// it has.
	DockerRunning func(ctx context.Context) bool
	// Info prints what the backend is doing.
	Info func(msg.M)
	// Warn prints a warning, and where the run log has more.
	Warn func(msg.M)
	// WithSteps numbers the logged commands of a phase of so many steps.
	WithSteps func(ctx context.Context, total int) context.Context
	// AddCleanup adds to the run's cleanup stack; Interrupted reports
	// whether the run was interrupted.
	AddCleanup  func(f func(context.Context)) (remove func())
	Interrupted func() bool
	// Forwards is how VMs' ports are forwarded: their records and
	// forwarders.
	Forwards forwards
}

// UI is where the backend prints, the run log its commands run into, and
// the run's cleanup stack.
type UI interface {
	ubuntu.UI
	Info(msg.M)
	Warn(msg.M)
	LogNote()
	WithSteps(ctx context.Context, total int) context.Context
	AddCleanup(f func(context.Context)) (remove func())
	Interrupted() bool
}

var (
	_ backend.Backend = (*Lima)(nil)
	_ backend.Storage = (*Lima)(nil)
)

// New returns the Lima backend for devvm's folders and its own files
// (assets), on u, sizing base to host.
func New(d paths.Dirs, assets fs.FS, u UI, host platform.Host) *Lima {
	self, err := os.Executable()
	if err == nil {
		if r, rerr := filepath.EvalSymlinks(self); rerr == nil {
			self = r
		}
	}
	return &Lima{
		Dirs: d, Runner: logRunner{u.LoggedCmd}, OS: runtime.GOOS, Arch: runtime.GOARCH,
		Self: self, Spawn: platform.StartDetachedProcess, Tick: 500 * time.Millisecond, Assets: assets, SSHTimeout: sshTimeout, OCIWait: time.Second, Getenv: os.Getenv,
		Ubuntu: ubuntu.New(assets, os.Getenv, u).Image, Memory: host.Memory, CPUs: host.CPUs,
		MacOS: platform.MacOSVersion, KVM: cmp.Or(os.Getenv("DEVVM_KVM"), "/dev/kvm"),
		DockerRunning: from.DockerRunning,
		Info:          u.Info, Warn: func(m msg.M) { u.Warn(m); u.LogNote() },
		WithSteps: u.WithSteps, AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
		Forwards: newForwards(d, self),
	}
}

// Name returns lima.
func (l *Lima) Name() string { return Name }

// assets returns the folder devvm's own files are written out in, for
// limactl create's base.yaml and the conversion VM's oci/.
func (l *Lima) assets() (string, error) { return state.WriteAssets(l.Dirs, l.Assets) }

// KeyFiles is the pinned Ubuntu cloud image base starts from, for the
// host's architecture.
func (l *Lima) KeyFiles() map[string][]byte {
	return map[string][]byte{"ubuntu-image": []byte(ubuntu.CloudPin(l.Arch))}
}

// DiskDirs is devvm's Lima home, which holds every image's and VM's disk
// (backend.Storage).
func (l *Lima) DiskDirs() []string { return []string{l.Dirs.Lima} }

// Floors is the free space Lima's steps want (backend.Storage), measured
// with statfs on macOS (vz, APFS) in the e2e sandbox. Base took 1.1 GiB
// at its peak with Ubuntu's cloud image already in Lima's cache; the
// download adds up to 0.9 GiB, and every feature took base's disk from
// 2.6 to 5.1 GiB (du). The docker layer took 0.6 GiB, a VM up to 0.1 GiB
// with its first launch, and shared tools builds 0.35 GiB at their peak
// for claude, 2.1 for rust and 5.9 for all ten. On Linux hosts (QEMU, not
// measured here) an image's and a tools build VM's disks start as full
// copies of their parent's (2.0 GB as qcow2 without features), while a
// VM's is an overlay of its image's.
func (l *Lima) Floors() backend.Floors {
	f := backend.Floors{Base: 8 << 30, Image: 2 << 30, VM: 2 << 30, Tools: 3 << 30, PerTool: 512 << 20}
	if l.overlays() {
		f.Image += 6 << 30
		f.Tools += 6 << 30
	}
	return f
}

// Caps is what Lima has: everything, natively, but its ports, which
// devvm's forwarder forwards (ports.go).
func (l *Lima) Caps() backend.Caps {
	caps := backend.Caps{}
	for _, f := range backend.Features {
		caps[f] = backend.Native
	}
	caps[backend.FeaturePorts] = backend.Emulated(true)
	caps[backend.FeaturePortsAll] = backend.Emulated(true)
	return caps
}

// socketMax is the longest unix socket path macOS takes (Linux takes 108).
// sshSocketExtra is what OpenSSH adds to the socket's name while making it.
const (
	socketMax      = 104
	sshSocketExtra = 17
)

// NameMax is the longest VM name whose ssh socket fits: Lima keeps it in
// the VM's folder (<Lima home>/<vm>/ssh.sock).
func (l *Lima) NameMax() int {
	return max(0, socketMax-sshSocketExtra-len(l.Dirs.Lima)-len("/")-len("/ssh.sock"))
}

// projectVM matches a project VM's name: devvm-<folder>-<14 hex>. A folder
// named img-… gives devvm-img-…-<hash>, so starting with ImagePrefix alone
// doesn't make an image.
var projectVM = regexp.MustCompile(`^devvm-.+-[0-9a-f]{14}$`)

// imageKey returns the key of an image's instance, and whether it is one.
func imageKey(name string) (string, bool) {
	key, ok := strings.CutPrefix(name, ImagePrefix)
	return key, ok && key != "" && !projectVM.MatchString(name)
}

// Images lists the keys of the images in devvm's Lima home.
func (l *Lima) Images(ctx context.Context) ([]string, error) {
	ins, err := l.list(ctx)
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, in := range ins {
		if k, ok := imageKey(in.Name); ok {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// ImageSize is the disk space of the image's instance folder: its disk
// and Lima's files. On macOS an image's disk is an APFS clone of its
// parent's, so the blocks they share count for both.
func (l *Lima) ImageSize(_ context.Context, key string) (int64, error) {
	name := ImagePrefix + key
	if _, ok := imageKey(name); !ok || strings.ContainsAny(key, "/\x00") {
		return 0, msg.NotAnImage(name)
	}
	return platform.DiskUsage(l.Dirs.LimaVM(name))
}

// DeleteImage deletes an image. On Linux hosts, an image whose disk
// project VMs are overlays of is refused.
func (l *Lima) DeleteImage(ctx context.Context, key string) error {
	name := ImagePrefix + key
	if _, ok := imageKey(name); !ok || strings.ContainsAny(key, "/\x00") {
		return msg.NotAnImage(name)
	}
	if _, err := l.inspect(ctx, name); errors.Is(err, backend.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	if l.overlays() {
		users, err := l.overlayUsers(ctx, name)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return msg.ImageInUse(key, strings.Join(users, " "))
		}
	}
	return l.delete(ctx, name)
}

// CreateVM clones the image's instance into a new, stopped VM. On Linux
// hosts the VM's disk is then an overlay of the image's.
func (l *Lima) CreateVM(ctx context.Context, image string, spec backend.VMSpec) error {
	vm := spec.Name
	if len(vm) > l.NameMax() {
		return msg.LimaHomeTooLong(l.Dirs.Lima, vm)
	}
	var ms []mount
	if spec.Project != "" {
		ms = append(ms, mount{Location: spec.Project, Writable: ptr(true)})
	}
	ms = append(ms, readOnly(spec.AgentDirs)...)
	if spec.Tools != "" {
		ms = append(ms, mount{Location: spec.Tools, MountPoint: ptr(spec.ToolsAt), Writable: ptr(false)})
	}
	mounts, err := mountsSet(ms)
	if err != nil {
		return err
	}
	src := ImagePrefix + image
	img, err := l.inspect(ctx, src)
	if errors.Is(err, backend.ErrNotFound) {
		return msg.ImageMissing(image)
	} else if err != nil {
		return err
	}
	if backend.Status(img.Status) != backend.StatusStopped {
		if err := l.stop(ctx, src); err != nil {
			return err
		}
	}
	// --mount-none: the image's own mounts (the template's home folder)
	// would otherwise stay. Lima applies --set after it. Its environment
	// too, so a start without network policy edits nothing (Start).
	args := []string{
		"clone", "--tty=false", src, vm, "--mount-none",
		"--set", mounts,
		"--set", ".ssh.forwardAgent=" + strconv.FormatBool(spec.AgentForwarding),
		"--set", ".ssh.localPort=" + strconv.Itoa(spec.SSHPort),
		"--set", noForwardsSet(),
	}
	if len(spec.Env) > 0 {
		args = append(args, "--set", ".env = "+yqJSON(spec.Env))
	}
	if err := l.Runner.Run(ctx, msg.Creating(vm), l.limactl(ctx, args...)); err != nil {
		return backend.Logged(msg.VMCreateFailed(vm))
	}
	if err := l.Forwards.write(vm, spec.Ports); err != nil {
		return err
	}
	if l.overlays() {
		if err := l.overlay(ctx, src, vm); err != nil {
			return err
		}
	}
	return l.SetSize(ctx, vm, spec.Size)
}

// Stop stops a VM; a stopped one is left as it is.
func (l *Lima) Stop(ctx context.Context, vm string) error {
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return err
	}
	if backend.Status(in.Status) == backend.StatusStopped {
		l.Forwards.Stop(vm)
		return nil
	}
	err = l.stop(ctx, vm)
	l.Forwards.Stop(vm)
	return err
}

func (l *Lima) stop(ctx context.Context, vm string) error {
	if err := l.Runner.Run(ctx, msg.Stopping(vm), l.limactl(ctx, "stop", vm)); err != nil {
		return backend.Logged(msg.StopFailed(vm))
	}
	return nil
}

// Delete deletes a VM, stopping it first, and its files in the run folder.
func (l *Lima) Delete(ctx context.Context, vm string) error {
	if _, err := l.inspect(ctx, vm); err != nil {
		return err
	}
	return l.delete(ctx, vm)
}

func (l *Lima) delete(ctx context.Context, vm string) error {
	if err := l.Runner.Run(ctx, msg.Deleting(vm), l.limactl(ctx, "delete", "-f", vm)); err != nil {
		return backend.Logged(msg.DeleteFailed(vm))
	}
	if err := l.Forwards.forget(vm); err != nil {
		return err
	}
	for _, f := range []string{l.profileFile(vm), l.startedFile(vm)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// List lists the VMs in devvm's Lima home, without images.
func (l *Lima) List(ctx context.Context) ([]backend.Instance, error) {
	ins, err := l.list(ctx)
	if err != nil {
		return nil, err
	}
	var out []backend.Instance
	for _, in := range ins {
		if _, ok := imageKey(in.Name); !ok {
			out = append(out, in.backend())
		}
	}
	return out, nil
}

// Inspect returns one VM.
func (l *Lima) Inspect(ctx context.Context, vm string) (backend.Instance, error) {
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return backend.Instance{}, err
	}
	return in.backend(), nil
}

const gib = 1 << 30

func (in instance) size() backend.Size {
	return backend.Size{CPUs: in.CPUs, MemoryGiB: float64(in.Memory) / gib, DiskGiB: float64(in.Disk) / gib}
}

func (in instance) backend() backend.Instance {
	b := backend.Instance{Name: in.Name, Status: backend.Status(in.Status), Size: in.size()}
	for _, m := range in.Config.Mounts {
		if m.agentDir() {
			b.AgentDirs = append(b.AgentDirs, m.Location)
		}
	}
	b.AgentForwarding = in.Config.SSH.ForwardAgent != nil && *in.Config.SSH.ForwardAgent
	return b
}

// Exec runs argv in the VM, in o.Dir. limactl shell passes each of its
// arguments to the guest's shell quoted; the command devvm builds for it
// (the folder, then argv) is quoted word by word too.
func (l *Lima) Exec(ctx context.Context, vm string, argv []string, o backend.ExecOpts) error {
	dir := o.Dir
	if dir == "" {
		dir = "/"
	}
	cmd := l.limactl(ctx, "shell", "--workdir", "/", vm, "sh", "-c", "cd "+shq(dir)+" && exec "+shqJoin(argv))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	return l.Runner.Attach(cmd)
}

// Shell returns limactl shell in dir, for the caller to run in its place.
func (l *Lima) Shell(ctx context.Context, vm, dir string) (backend.Command, error) {
	cmd := l.limactl(ctx, "shell", "--workdir", dir, vm)
	if cmd.Err != nil {
		return backend.Command{}, cmd.Err
	}
	return backend.Command{Path: cmd.Path, Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir}, nil
}

// sshLogin is the options of Lima's ssh config, lowercase, that say how
// to log in. Anything else Lima (or a later version) writes there stays
// out: it turns host key checks off and shares one connection.
var sshLogin = []string{
	"identityfile", "identitiesonly", "preferredauthentications", "batchmode",
	"compression", "gssapiauthentication", "ciphers",
}

// SSH returns how ssh reaches a running VM: the address, port, user and
// login options in the ssh config Lima keeps for it, which has its key.
func (l *Lima) SSH(ctx context.Context, vm string) (backend.SSHTarget, error) {
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return backend.SSHTarget{}, err
	}
	if backend.Status(in.Status) != backend.StatusRunning {
		return backend.SSHTarget{}, msg.VMNotRunning(vm)
	}
	t := backend.SSHTarget{Host: "127.0.0.1", Port: in.SSHLocalPort, ConfigFile: in.SSHConfigFile}
	if in.SSHConfigFile == "" {
		return t, nil
	}
	f, err := os.Open(in.SSHConfigFile)
	if err != nil {
		return backend.SSHTarget{}, err
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	for s.Scan() {
		k, raw, _ := strings.Cut(strings.TrimSpace(s.Text()), " ")
		raw = strings.TrimSpace(raw)
		v := strings.Trim(raw, `"`)
		switch lk := strings.ToLower(k); {
		case lk == "hostname":
			t.Host = v
		case lk == "port":
			if p, err := strconv.Atoi(v); err == nil {
				t.Port = p
			}
		case lk == "user":
			t.User = v
		case slices.Contains(sshLogin, lk) && raw != "":
			t.Options = append(t.Options, backend.SSHOption{Key: k, Value: raw})
		}
	}
	return t, s.Err()
}

// stopped returns a VM that must be stopped to be changed.
func (l *Lima) stopped(ctx context.Context, vm string) (instance, error) {
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return instance{}, err
	}
	if backend.Status(in.Status) != backend.StatusStopped {
		return instance{}, msg.VMNotStopped(vm)
	}
	return in, nil
}

// SetSize changes a stopped VM's size where it differs. Lima can't shrink a
// disk, so a smaller one is refused.
func (l *Lima) SetSize(ctx context.Context, vm string, s backend.Size) error {
	if s == (backend.Size{}) {
		return nil
	}
	in, err := l.stopped(ctx, vm)
	if err != nil {
		return err
	}
	cur := in.size()
	var opts []string
	if s.CPUs > 0 && s.CPUs != cur.CPUs {
		opts = append(opts, "--cpus", strconv.Itoa(s.CPUs))
	}
	if s.MemoryGiB > 0 && s.MemoryGiB != cur.MemoryGiB {
		opts = append(opts, "--memory", gibString(s.MemoryGiB))
	}
	if s.DiskGiB > 0 && s.DiskGiB < cur.DiskGiB {
		return msg.DiskShrink(gibString(cur.DiskGiB), vm, gibString(s.DiskGiB))
	}
	if s.DiskGiB > cur.DiskGiB {
		opts = append(opts, "--disk", gibString(s.DiskGiB))
	}
	if len(opts) == 0 {
		return nil
	}
	return l.edit(ctx, vm, opts...)
}

func gibString(g float64) string { return strconv.FormatFloat(g, 'g', -1, 64) }

// SetAgentDirs replaces a stopped VM's agent folder mounts, keeping its
// other mounts (the project folder, the tools share).
func (l *Lima) SetAgentDirs(ctx context.Context, vm string, dirs []string) error {
	in, err := l.stopped(ctx, vm)
	if err != nil {
		return err
	}
	var have []string
	var ms []mount
	for _, m := range in.Config.Mounts {
		if m.agentDir() {
			have = append(have, m.Location)
		} else {
			ms = append(ms, m)
		}
	}
	if slices.Equal(have, dirs) {
		return nil
	}
	set, err := mountsSet(append(ms, readOnly(dirs)...))
	if err != nil {
		return err
	}
	return l.edit(ctx, vm, "--set", set)
}

// SetAgentForwarding turns a stopped VM's ssh-agent forwarding on or off.
func (l *Lima) SetAgentForwarding(ctx context.Context, vm string, on bool) error {
	in, err := l.stopped(ctx, vm)
	if err != nil {
		return err
	}
	if in.backend().AgentForwarding == on {
		return nil
	}
	return l.edit(ctx, vm, "--set", ".ssh.forwardAgent="+strconv.FormatBool(on))
}

// overlays reports whether project VMs' disks are overlays of their
// image's: on Linux hosts, where Lima's QEMU disks are qcow2 files and a
// clone would be a full copy. (On macOS clones are APFS copies, which
// share blocks already.)
func (l *Lima) overlays() bool { return l.OS == "linux" }

func (l *Lima) disk(vm string) string { return filepath.Join(l.Dirs.LimaVM(vm), "disk") }

func qemuImg(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "qemu-img", args...) //nolint:gosec // devvm's own arguments
}

// qemuImgInfo is what qemu-img info --output=json says about a disk.
type qemuImgInfo struct {
	Format  string `json:"format"`
	Backing string `json:"backing-filename"`
}

func (l *Lima) diskInfo(ctx context.Context, path string) (qemuImgInfo, error) {
	var info qemuImgInfo
	out, err := l.Runner.Output(qemuImg(ctx, "info", "--output=json", path))
	if err != nil {
		return info, fmt.Errorf("qemu-img info %s: %w", path, err)
	}
	return info, json.Unmarshal(out, &info)
}

// overlay replaces vm's copy of src's disk with a qcow2 overlay of it: the
// image's blocks are shared and the VM's disk holds only its changes. A
// disk that isn't qcow2 (Lima's layout changed) is kept as it is.
func (l *Lima) overlay(ctx context.Context, src, vm string) error {
	img, disk := l.disk(src), l.disk(vm)
	if fi, err := os.Stat(img); err != nil || !fi.Mode().IsRegular() {
		return nil //nolint:nilerr // no qcow2 disk to share: keep the copy
	}
	info, err := l.diskInfo(ctx, disk)
	if err != nil {
		return backend.Logged(msg.VMCreateFailed(vm))
	}
	if info.Format != "qcow2" {
		return nil
	}
	if err := os.Remove(disk); err != nil {
		return err
	}
	if err := l.Runner.Run(ctx, msg.M{}, qemuImg(ctx, "create", "-q", "-f", "qcow2", "-b", img, "-F", "qcow2", disk)); err != nil {
		return backend.Logged(msg.VMCreateFailed(vm))
	}
	return os.Chmod(disk, 0o600) // it may hold tokens: owner only
}

// overlayUsers returns the VMs whose disks are overlays of image's.
func (l *Lima) overlayUsers(ctx context.Context, image string) ([]string, error) {
	ins, err := l.list(ctx)
	if err != nil {
		return nil, err
	}
	var users []string
	for _, in := range ins {
		if _, ok := imageKey(in.Name); ok {
			continue
		}
		if _, err := os.Stat(l.disk(in.Name)); err != nil {
			continue
		}
		info, err := l.diskInfo(ctx, l.disk(in.Name))
		if err != nil {
			return nil, err
		}
		if info.Backing != "" && filepath.Clean(info.Backing) == l.disk(image) {
			users = append(users, in.Name)
		}
	}
	return users, nil
}
