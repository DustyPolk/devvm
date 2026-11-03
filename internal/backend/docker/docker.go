// Package docker is the Docker backend (experimental): each VM is a
// container, which runs its image with no init system, from images devvm
// builds by running its scripts in a container and committing it. A
// container is not a VM: on Linux every container shares the host's
// kernel, and on macOS Docker Desktop runs them all in one VM.
//
// Docker's engine is the user's own (Docker Desktop's especially), so
// everything devvm makes in it carries a label naming devvm's data folder
// (LabelKey), and devvm never starts, changes or deletes a container,
// image or volume without it. Names Docker keeps for every user (image
// tags, the tools volume) also carry a hash of that folder.
//
// What Docker fixes when it makes a container (its mounts, its network)
// is compared at each start with what the VM wants; where they differ the
// container is committed and made again from that commit, under the same
// name (start.go). The VM's home is a volume of its own, kept across that.
//
// ssh for editors (ssh.go), forwarding ports (ports.go) and SSH agent
// forwarding, chosen when a VM is made and changed later (its ssh
// sessions', agent.go) are emulated, all over one transport: docker exec
// into the container, to an sshd of its own per connection; all reviewed.
// Network policy, and build confinement on Docker Desktop, are emulated
// too (egress.go): an internal network of the VM's own, with a relay to
// devvm's proxy on the host; reviewed. The docker layer runs only in a VM
// made for it (dockerlayer.go), under Sysbox where the engine has it, and
// nowhere else: emulated, reviewed, with no network policy.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// Name is the backend's name.
const Name = "docker"

// Labels devvm puts on what it makes in Docker.
const (
	// LabelKey is on every container, image and volume devvm makes, its
	// value devvm's data folder.
	LabelKey = "devvm.data"
	// kindLabel says what a container or image is: kindVM, kindBuild,
	// kindTools or kindImage.
	kindLabel = "devvm.kind"
	// keyLabel is an image's key, on the image and its build container.
	keyLabel = "devvm.key"
	// configLabel is a VM's container's config hash (config.hash): what it
	// was made with.
	configLabel = "devvm.config"
)

// Kinds of what devvm makes, as kindLabel has them: containers (a VM's,
// an image's build, the shared tools' build or unpacking) and images (an
// image of devvm's, a VM's own commit).
const (
	kindVM    = "vm"
	kindBuild = "build"
	kindTools = "tools"
	kindImage = "image"
	kindDisk  = "disk"
)

// Paths in containers.
const (
	// toolsAt is where project VMs have the tools volume, read-only.
	toolsAt = "/mnt/devvm-tools"
	// shareAt and storeAt are where the tools' unpacking container has the
	// share (or a build's output folder), read-only, and the tools volume.
	shareAt = "/mnt/devvm-share"
	storeAt = "/mnt/devvm-store"
)

// Docker is the backend. New sets every field; tests change them.
type Docker struct {
	// client runs docker, on what this devvm makes: Dirs.Data is its
	// Data.
	client
	Dirs paths.Dirs
	// OS and Arch are the host's, as Go names them.
	OS, Arch string
	// Assets holds devvm's own files (assets.FS): base.yaml (base's CPUs,
	// memory and disk), container/ (the scripts run as root in containers
	// it shares with smolvm) and docker/ (its own).
	Assets fs.FS
	// User is the host's user, whose name, uid and gid the VM's user gets.
	User func() (*user.User, error)
	// Hostname is the host's name, which a Docker engine on this machine
	// has too.
	Hostname func() (string, error)
	// Terminal reports whether devvm's input is a terminal (a shell gets
	// one then).
	Terminal func() bool
	// SharingFile is Docker Desktop's settings file, which lists the host
	// folders it shares with containers; "" for none to read.
	SharingFile string
	// Kind is the engine's kind (kindDesktop, kindLinux, kindOther) when
	// set: tests', and test builds' (DEVVM_DOCKER_KIND), so builds run on a
	// stand-in engine anywhere; "" asks the engine.
	Kind string
	// Self is devvm's binary, as it was run: ssh's ProxyCommand for a VM
	// runs it (ConnectCmd), and so does each VM's port forwarder, and a
	// Docker Desktop build's proxy.
	Self string
	// Linux returns the path of devvm's Linux build for this machine's
	// processor, which network policy's relays run (linux.go): with
	// fetch, fetching a release's own when it isn't there yet.
	Linux func(ctx context.Context, fetch bool) (string, error)
	// LinuxURL and LinuxSHA256 are a release's Linux build: its archive
	// and its devvm's SHA-256, which a release sets (registry); "" for
	// none to fetch. LinuxOS, when set, is the host's system as finding
	// it has it (test builds': darwin, to fetch on any host).
	LinuxURL, LinuxSHA256, LinuxOS string
	// Detach starts a process that outlives devvm (daemon.Detach): a VM's
	// lifeline.
	Detach daemon.Starter
	// Spawn starts a process that can outlive devvm and returns it
	// (daemon.Spawn): a build's proxy, stopped through it, so never
	// another process given its pid.
	Spawn func(daemon.Cmd) (*daemon.Handle, error)
	// Tick is how often a build's proxy is looked at while it starts, ten
	// times at most.
	Tick time.Duration
	// ProcSys is where this machine's /proc/sys is: a Linux engine's
	// bridges' settings are read there (dockerlayer.go). Test builds' is
	// DEVVM_DOCKER_PROC_SYS, for the CLI tests' stand-in engine.
	ProcSys string
	// Getenv reads the environment (connectEnv: DOCKER_HOST…), and
	// LookPath finds a command on PATH (docker).
	Getenv   func(string) string
	LookPath func(string) (string, error)
	// Forwards runs the port forwarders of VMs that forward ports.
	Forwards vmssh.Forwards
	// Info prints what the backend is doing; Warn prints a warning.
	Info, Warn func(msg.M)
	// WithSteps numbers the logged commands of a phase of so many steps.
	WithSteps func(ctx context.Context, total int) context.Context
	// AddCleanup adds to the run's cleanup stack; Interrupted reports
	// whether the run was interrupted.
	AddCleanup  func(f func(context.Context)) (remove func())
	Interrupted func() bool

	// mu guards eng, the engine as Problems last found it.
	mu  sync.Mutex
	eng *engine
}

// UI is where the backend prints, the run log its commands run into, the
// run's cleanup stack, and whether devvm's input is a terminal.
type UI interface {
	Info(msg.M)
	Warn(msg.M)
	StdinIsTerminal() bool
	LoggedCmd(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	WithSteps(ctx context.Context, total int) context.Context
	AddCleanup(f func(context.Context)) (remove func())
	Interrupted() bool
}

var _ backend.Backend = (*Docker)(nil)

// New returns the Docker backend for devvm's folders and its own files
// (assets), on u.
func New(d paths.Dirs, assets fs.FS, u UI) *Docker {
	sharing := ""
	if runtime.GOOS == "darwin" {
		sharing = filepath.Join(d.Home, "Library", "Group Containers", "group.com.docker", "settings-store.json")
	}
	// As run, a link's path kept: devvm's ssh config names it, and a link
	// (Homebrew's) outlives the version it points to.
	self, _ := os.Executable()
	dk := &Docker{
		client: client{Runner: logRunner{u.LoggedCmd}, names: names{Data: d.Data}}, Dirs: d, OS: runtime.GOOS, Arch: runtime.GOARCH, Assets: assets,
		User: user.Current, Hostname: os.Hostname,
		Terminal: u.StdinIsTerminal, SharingFile: sharing,
		Self: self, Detach: daemon.Detach, Spawn: daemon.Spawn, Tick: 500 * time.Millisecond,
		ProcSys: "/proc/sys", Getenv: os.Getenv, LookPath: exec.LookPath, Forwards: newForwards(d, self),
		Info: u.Info, Warn: u.Warn,
		WithSteps: u.WithSteps, AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
	}
	// LinuxURL and LinuxSHA256 as they are when it's asked: the registry
	// sets them.
	dk.Linux = func(ctx context.Context, fetch bool) (string, error) {
		goos := dk.OS
		if dk.LinuxOS != "" {
			goos = dk.LinuxOS
		}
		return linuxFinder{
			Self: dk.Self, GOOS: goos, GOARCH: dk.Arch, URL: dk.LinuxURL, SHA256: dk.LinuxSHA256,
			Copy: linuxCopy(dk.Dirs, dk.Arch), Client: newLinuxClient(), RetryDelay: time.Second, Info: dk.Info,
		}.find(ctx, fetch)
	}
	return dk
}

// Name returns docker.
func (d *Docker) Name() string { return Name }

// Caps is what Docker has natively: building images by committing
// containers, --from any image Docker has or pulls, containers from
// images, bind mounts, the tools in a volume, CPUs and memory. Build
// confinement is native on a Linux engine on this machine, whose
// containers don't reach its loopback; on Docker Desktop, whose
// containers reach the host's, it's emulated (egress.go), as network
// policy is on both, all reviewed. Neither on another engine, which can't
// reach this machine's proxy. ssh for editors, ports
// (listed and every one) and agent forwarding (and turning it on or off)
// are emulated over docker exec, and reviewed. The docker layer is
// emulated under Sysbox where a Linux engine of this machine's has it, and reviewed
// (dockerlayer.go). The rest are none: the nix layer among them, because
// its containers can't make the kernel namespaces Nix's build sandbox
// needs, and devvm's Nix builds nothing outside it.
func (d *Docker) Caps() backend.Caps {
	caps := backend.Caps{
		backend.FeatureSSH: backend.Emulated(true), backend.FeaturePorts: backend.Emulated(true),
		backend.FeaturePortsAll: backend.Emulated(true), backend.FeatureAgentForwarding: backend.Emulated(true),
		backend.FeatureAgentToggle: backend.Emulated(true),
	}
	if hasSysbox(d) && d.engineKind() == kindLinux {
		caps[backend.FeatureDockerLayer] = backend.Emulated(true)
	}
	for _, f := range []backend.Feature{
		backend.FeatureBuildImage, backend.FeatureFrom, backend.FeatureClone, backend.FeatureProjectMount,
		backend.FeatureAgentFolders, backend.FeatureToolsShare, backend.FeatureCPUs, backend.FeatureMemory,
		backend.FeatureLink,
	} {
		caps[f] = backend.Native
	}
	switch d.engineKind() {
	case kindLinux:
		caps[backend.FeatureBuildConfinement] = backend.Native
		caps[backend.FeatureEgressProxy] = backend.Emulated(true)
	case kindDesktop:
		caps[backend.FeatureBuildConfinement] = backend.Emulated(true)
		caps[backend.FeatureEgressProxy] = backend.Emulated(true)
	}
	return caps
}

// NameMax is 0: Docker's names have no limit devvm meets.
func (d *Docker) NameMax() int { return 0 }

// script is one of devvm's files: a path in Assets.
func (d *Docker) script(name string) ([]byte, error) { return fs.ReadFile(d.Assets, name) }

// KeyFiles are the scripts base's containers run that go into base, which
// smolvm's machines run too, one missing hashed as empty, and Ubuntu's
// pinned image.
func (d *Docker) KeyFiles() map[string][]byte {
	files := map[string][]byte{"ubuntu-image": []byte(ubuntu.ContainerRef)}
	for _, n := range []string{"container/prepare.sh", "container/shims.sh", "container/sshd.sh"} {
		files[n], _ = d.script(n)
	}
	return files
}

// ToolFormat is unpacked folders (docker/unpack.sh), as containers mount
// no images: its name changes with the script.
func (d *Docker) ToolFormat() string {
	unpack, _ := d.script("docker/unpack.sh") // missing, BuildTools fails
	sum := sha256.Sum256(unpack)
	return "dir-" + hex.EncodeToString(sum[:])[:12]
}

// homeVolume is a VM's home folder, and cloneVolume a --clone VM's clone.
func homeVolume(vm string) string  { return vm + "-home" }
func cloneVolume(vm string) string { return vm + "-clone" }

// nextName is a VM's next container, while it's made.
func nextName(vm string) string { return vm + "-next" }

// record is what devvm set on one of its VMs, kept in devvm's data folder
// (paths.Dirs.DockerVM): what its container is made with, and which commit
// of its own it runs.
type record struct {
	Image     string   `json:"image"` // its image's key
	Project   string   `json:"project,omitempty"`
	CloneAt   string   `json:"clone_at,omitempty"`
	AgentDirs []string `json:"agent_dirs,omitempty"`
	Tools     bool     `json:"tools,omitempty"` // it mounts the tools volume
	// Commit is the commit of its own its container runs, Gen's: "" for
	// one made from its image.
	Commit string `json:"commit,omitempty"`
	Gen    int    `json:"gen,omitempty"`
	// Confined is its container's ID and start time at its last start by
	// Start through devvm's proxy, NetID its own network's ID and RelayID
	// its relay's container's ID then (egress.go); "" for a start without.
	Confined string `json:"confined,omitempty"`
	NetID    string `json:"net_id,omitempty"`
	RelayID  string `json:"relay_id,omitempty"`
	// Forwarding is whether its ssh sessions forward an SSH agent (agent.go);
	// Agent is whether its last start's do: none (""), or the one devvm's
	// SSH_AUTH_SOCK names (vmssh.AgentUser).
	Forwarding bool   `json:"forwarding,omitempty"`
	Agent      string `json:"agent,omitempty"`
	// PortsAll is set when it forwards every port, else Ports are those it
	// lists (none when empty), through its forwarder (ports.go).
	PortsAll bool                `json:"ports_all,omitempty"`
	Ports    []backend.PortRange `json:"ports,omitempty"`
	// Started is its container's start time at its last start by Start,
	// with ssh once SSHUp is set (its sshd's config written, and cleared
	// before each start); NoSSHD is set when that start found no sshd in
	// its image (ssh.go).
	Started string `json:"started,omitempty"`
	SSHUp   bool   `json:"ssh_up,omitempty"`
	NoSSHD  bool   `json:"no_sshd,omitempty"`
	// Runtime is the runtime of a VM made for the docker layer, Sysbox's
	// (sysboxRuntime), with volumes of its own for Docker's engine
	// (dockerlayer.go); "" for any other.
	Runtime string `json:"runtime,omitempty"`
}

// dockerLayer reports whether the VM was made for the docker layer.
func (r record) dockerLayer() bool { return r.Runtime != "" }

func (d *Docker) readRecord(vm string) (record, error) {
	var r record
	b, err := os.ReadFile(d.Dirs.VM(vm).Record())
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	} else if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (d *Docker) writeRecord(vm string, r record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFile(d.Dirs.VM(vm).Record(), append(b, '\n'))
}

// List lists devvm's VMs: its containers labelled as VMs.
func (d *Docker) List(ctx context.Context) ([]backend.Instance, error) {
	cs, err := d.containers(ctx, kindVM)
	if err != nil {
		return nil, err
	}
	var out []backend.Instance
	for _, c := range cs {
		if strings.HasSuffix(c.name(), "-next") {
			continue // a VM's next container, while it's made
		}
		in, err := d.instance(c)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

// Inspect returns one of devvm's VMs.
func (d *Docker) Inspect(ctx context.Context, vm string) (backend.Instance, error) {
	c, err := d.vm(ctx, vm)
	if err != nil {
		return backend.Instance{}, err
	}
	return d.instance(c)
}

func (d *Docker) instance(c container) (backend.Instance, error) {
	r, err := d.readRecord(c.name())
	if err != nil {
		return backend.Instance{}, err
	}
	return backend.Instance{
		Name: c.name(), Status: c.status(), Size: c.size(),
		AgentDirs: slices.Clone(r.AgentDirs), AgentForwarding: r.Forwarding,
	}, nil
}

// Stop stops one of devvm's VMs, its port forwarder and its relay to
// devvm's proxy; a stopped one is left as it is.
func (d *Docker) Stop(ctx context.Context, vm string) error {
	c, err := d.vm(ctx, vm)
	if err != nil {
		return err
	}
	d.Forwards.Stop(vm)
	if c.status() != backend.StatusStopped {
		if err := d.stop(ctx, vm); err != nil {
			return err
		}
	}
	return egress{d}.stopRelay(ctx, vm)
}

// Delete deletes one of devvm's VMs, stopping it first: its container,
// its volumes (its Docker engine's too), its own commit, its relay and
// network (egress.go), then its forwarder and backend folder, with its
// keys and record.
func (d *Docker) Delete(ctx context.Context, vm string) error {
	c, err := d.vm(ctx, vm)
	if err != nil {
		return err
	}
	if err := (egress{d}).checkAlone(ctx, vm, c.ID); err != nil {
		return err
	}
	r, err := d.readRecord(vm)
	if err != nil {
		return err
	}
	d.Forwards.Stop(vm)
	if err := d.removeContainer(ctx, msg.Deleting(vm), vm); err != nil {
		return err
	}
	if err := d.removeLeaves(ctx, vm, r); err != nil {
		return err
	}
	return d.Forwards.Forget(vm)
}

// removeLeaves deletes what a VM has in its Docker engine besides its
// container: its relay and network (egress.go), its volumes (each only if
// it has devvm's label) and the image its last change was committed to,
// as its record r has it.
func (d *Docker) removeLeaves(ctx context.Context, vm string, r record) error {
	if err := (egress{d}).forget(ctx, vm); err != nil {
		return err
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

// user is the VM's user: the host's name, uid and gid.
func (d *Docker) user() (name, uid, gid string, err error) {
	u, err := d.User()
	if err != nil {
		return "", "", "", err
	}
	return u.Username, u.Uid, u.Gid, nil
}

func home(name string) string { return "/home/" + name + ".guest" }

// loginEnv is what runs a command as the VM's user would get from logging
// in: docker exec sets only what it's told, and reads no /etc/environment,
// which devvm's scripts write to (BASH_ENV).
const loginEnv = `set -a; [ -r /etc/environment ] && . /etc/environment; set +a; exec "$@"`

// userArgs are docker's arguments that run argv as the VM's user in a
// running container, in dir; in a build's container, with its proxy
// settings (execEnv).
func (d *Docker) userArgs(ctx context.Context, vm, dir string, input, tty bool, argv []string) ([]string, error) {
	name, _, _, err := d.user()
	if err != nil {
		return nil, err
	}
	if dir == "" {
		dir = "/"
	}
	args := append([]string{"exec"}, execEnvArgs(ctx)...)
	if input {
		args = append(args, "-i")
	}
	if tty {
		args = append(args, "-t")
	}
	args = append(args, "-u", name, "-w", dir,
		"-e", "USER="+name, "-e", "LOGNAME="+name, "-e", "HOME="+home(name), "-e", "SHELL=/bin/bash",
		vm, "sh", "-c", loginEnv, "sh")
	return append(args, argv...), nil
}

// Exec runs argv in the VM as its user, in o.Dir. Docker passes each
// argument as it is: no shell reads them.
func (d *Docker) Exec(ctx context.Context, vm string, argv []string, o backend.ExecOpts) error {
	args, err := d.userArgs(ctx, vm, o.Dir, o.Stdin != nil, false, argv)
	if err != nil {
		return err
	}
	cmd := d.docker(ctx, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	return d.Runner.Attach(cmd)
}

// Shell returns a login shell in dir as the VM's user, with a terminal when
// devvm has one: over its ssh when its start ran its sshd (an ssh session,
// with the agent that start chose, agent.go), else through docker exec.
func (d *Docker) Shell(ctx context.Context, vm, dir string) (backend.Command, error) {
	sshArgs, ok, err := d.sshShell(ctx, vm, dir)
	if err != nil {
		return backend.Command{}, err
	}
	if ok {
		path, err := d.LookPath("ssh")
		if err != nil {
			return backend.Command{}, err
		}
		return backend.Command{Path: path, Args: append([]string{"ssh"}, sshArgs...), Env: os.Environ(), Dir: d.Data}, nil
	}
	args, err := d.userArgs(ctx, vm, dir, true, d.Terminal(), []string{"bash", "-l"})
	if err != nil {
		return backend.Command{}, err
	}
	cmd := d.docker(ctx, args...)
	if cmd.Err != nil {
		return backend.Command{}, cmd.Err
	}
	return backend.Command{Path: cmd.Path, Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir}, nil
}

// ProxyURL is devvm's proxy as a VM started with c reaches it: its relay,
// devvm-proxy on its own network, on c's port, with the user and
// password; "" unless c.Net is NetProxy.
func (d *Docker) ProxyURL(c backend.Confinement) string {
	if c.Net != backend.NetProxy {
		return ""
	}
	return backend.ProxyURL(proxyAlias, c.ProxyPort, c.ProxyPassword)
}

// ProxySocket is where a VM's proxy listens for its relay (egress.go).
func (d *Docker) ProxySocket(vm string) string { return proxySocket(d.Dirs, vm) }

// Confined reports whether a running VM runs as Start left it through
// devvm's proxy: that start of its container, on its own network alone,
// which still confines (egress.go).
func (d *Docker) Confined(ctx context.Context, vm string) (bool, error) {
	return egress{d}.confined(ctx, vm)
}

// Lifeline is the pid file of a VM's docker container wait, which a start
// through devvm's proxy runs and which ends when the container stops.
func (d *Docker) Lifeline(_ context.Context, vm string) string { return d.Dirs.VM(vm).Lifeline() }

// StartID is a running VM's container and the time it last started.
func (d *Docker) StartID(ctx context.Context, vm string) (string, error) {
	c, err := d.vm(ctx, vm)
	if err != nil || c.status() != backend.StatusRunning {
		return "", err
	}
	return c.ID + " " + c.State.StartedAt, nil
}

// stopped returns a VM that must be stopped to be changed, and its record.
func (d *Docker) stopped(ctx context.Context, vm string) (container, record, error) {
	c, err := d.vm(ctx, vm)
	if err != nil {
		return container{}, record{}, err
	}
	if c.status() != backend.StatusStopped {
		return container{}, record{}, msg.VMNotStopped(vm)
	}
	r, err := d.readRecord(vm)
	return c, r, err
}

// SetSize changes a stopped VM's CPUs and memory where they differ. A disk
// size is refused: Docker has none to set.
func (d *Docker) SetSize(ctx context.Context, vm string, sz backend.Size) error {
	if sz.DiskGiB > 0 {
		return msg.BackendLacks(Name, string(backend.FeatureDisk), vm)
	}
	if sz == (backend.Size{}) {
		return nil
	}
	c, _, err := d.stopped(ctx, vm)
	if err != nil {
		return err
	}
	cur := c.size()
	if sz.CPUs == 0 || sz.CPUs == cur.CPUs {
		sz.CPUs = 0
	}
	if sz.MemoryGiB == 0 || sz.MemoryGiB == cur.MemoryGiB {
		sz.MemoryGiB = 0
	}
	opts := sizeArgs(sz.CPUs, sz.MemoryGiB)
	if len(opts) == 0 {
		return nil
	}
	if err := d.Runner.Run(ctx, msg.Configuring(vm), d.docker(ctx, append(append([]string{"update"}, opts...), vm)...)); err != nil {
		return backend.Logged(msg.DockerUpdateFailed(vm))
	}
	return nil
}

// SetAgentDirs replaces a stopped VM's agent folder mounts, from its next
// start: its container is made again then (start.go).
func (d *Docker) SetAgentDirs(ctx context.Context, vm string, dirs []string) error {
	_, r, err := d.stopped(ctx, vm)
	if err != nil {
		return err
	}
	if slices.Equal(r.AgentDirs, dirs) {
		return nil
	}
	r.AgentDirs = slices.Clone(dirs)
	return d.writeRecord(vm, r)
}
