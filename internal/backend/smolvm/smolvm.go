// Package smolvm is the smolvm backend (experimental): each VM is a smolvm
// machine, a microVM running its image as a container, with no init
// system. Images are packs (smolvm pack create --from-vm) in devvm's data
// folder, and a project VM is a machine made from its image's pack.
//
// On macOS smolvm keeps every machine in the user's own state, so devvm's
// carry a label naming devvm's data folder, and devvm never stops, changes
// or deletes a machine without it. On Linux smolvm's state is devvm's own
// (--state-dir) as well.
//
// Network policy is emulated, reviewed (start.go); ssh for editors is
// emulated, reviewed (ssh.go); forwarding ports, every one or the listed
// ones, is emulated, reviewed (forward.go, vmssh.Forwarder); turning agent
// forwarding on or off later is emulated, reviewed (agent.go).
package smolvm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// Name is the backend's name.
const Name = "smolvm"

// ImagePrefix starts the name of an image's build machine:
// devvm-img-<key>. Once built, the image is its pack.
const ImagePrefix = "devvm-img-"

// LabelKey is the label every machine devvm makes carries, its value
// devvm's data folder.
const LabelKey = "devvm.data"

// Gateway is the host as a machine reaches it.
const Gateway = "100.96.0.1"

// Smolvm is the backend. New sets every field; tests change them.
type Smolvm struct {
	Dirs   paths.Dirs
	Runner Runner
	// OS and Arch are the host's, as Go names them.
	OS, Arch string
	// Assets holds devvm's own files (assets.FS): base.yaml (base's CPUs,
	// memory and disk) and the scripts smolvm/ has, run as root in
	// machines.
	Assets fs.FS
	// Getenv reads the environment (SSH_AUTH_SOCK).
	Getenv func(string) string
	// User is the host's user, whose name and uid the VM's user gets.
	User func() (*user.User, error)
	// Memory returns the host's memory in bytes.
	Memory func() (uint64, error)
	// CPUs returns the host's CPU count.
	CPUs func() int
	// KVM is Linux's KVM device: /dev/kvm, unless a test build stands in
	// for it.
	KVM string
	// Terminal reports whether devvm's input is a terminal (a shell gets
	// one then).
	Terminal func() bool
	// FreePort returns a port free on the host's loopback, and PortFree
	// reports whether one is: for a VM's ssh forward.
	FreePort func(context.Context) (int, error)
	PortFree func(context.Context, int) bool
	// Info prints what the backend is doing.
	Info func(msg.M)
	// Warn prints a warning.
	Warn func(msg.M)
	// WithSteps numbers the logged commands of a phase of so many steps.
	WithSteps func(ctx context.Context, total int) context.Context
	// AddCleanup adds to the run's cleanup stack; Interrupted reports
	// whether the run was interrupted.
	AddCleanup  func(f func(context.Context)) (remove func())
	Interrupted func() bool
	// Forwards runs the port forwarders of VMs that forward every port.
	Forwards vmssh.Forwards
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

var (
	_ backend.Backend = (*Smolvm)(nil)
	_ backend.Storage = (*Smolvm)(nil)
)

// New returns the smolvm backend for devvm's folders and its own files
// (assets), on u, sizing base to host.
func New(d paths.Dirs, assets fs.FS, u UI, host platform.Host) *Smolvm {
	self, err := os.Executable()
	if err == nil {
		if r, rerr := filepath.EvalSymlinks(self); rerr == nil {
			self = r
		}
	}
	return &Smolvm{
		Dirs: d, Runner: logRunner{u.LoggedCmd}, OS: runtime.GOOS, Arch: runtime.GOARCH, Assets: assets,
		Getenv: os.Getenv, User: user.Current, Memory: host.Memory, CPUs: host.CPUs,
		KVM:      "/dev/kvm",
		Terminal: u.StdinIsTerminal,
		FreePort: freePort, PortFree: platform.PortFree,
		Info: u.Info, Warn: u.Warn,
		WithSteps: u.WithSteps, AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
		Forwards: newForwards(d, self),
	}
}

// Name returns smolvm.
func (s *Smolvm) Name() string { return Name }

// Caps is what smolvm has natively, egress through devvm's proxy emulated
// and reviewed (Start: an allow list of the host, and the proxy deciding
// each connection), ssh for editors emulated and reviewed (an sshd of
// devvm's, ssh.go), and ports forwarded, every one or the listed ones,
// emulated and reviewed (a forwarder of devvm's over that ssh,
// forward.go). Agent forwarding turned on or off once a VM is made is
// emulated and reviewed (every VM made forwarding, an agent chosen at each
// start, agent.go).
func (s *Smolvm) Caps() backend.Caps {
	caps := backend.Caps{
		backend.FeatureEgressProxy: backend.Emulated(true), backend.FeatureSSH: backend.Emulated(true),
		backend.FeaturePorts: backend.Emulated(true), backend.FeaturePortsAll: backend.Emulated(true),
		backend.FeatureAgentToggle: backend.Emulated(true),
	}
	for _, f := range []backend.Feature{
		backend.FeatureBuildImage, backend.FeatureBuildConfinement, backend.FeatureFrom, backend.FeatureClone,
		backend.FeatureProjectMount, backend.FeatureAgentFolders, backend.FeatureToolsShare,
		backend.FeatureAgentForwarding, backend.FeatureCPUs, backend.FeatureMemory, backend.FeatureDisk,
		backend.FeatureDockerLayer, backend.FeatureNixLayer, backend.FeatureLink,
	} {
		caps[f] = backend.Native
	}
	return caps
}

// UsesDecider is true: a machine with network policy reaches devvm's proxy
// only through its decider (smolvm's mediated egress).
func (s *Smolvm) UsesDecider() bool { return true }

// NameMax is 0: smolvm keeps a machine's sockets in a folder named by a
// hash, so names have no limit devvm meets.
func (s *Smolvm) NameMax() int { return 0 }

// script is one of devvm's scripts for smolvm: smolvm/<name> in Assets.
func (s *Smolvm) script(name string) ([]byte, error) { return fs.ReadFile(s.Assets, "smolvm/"+name) }

// containerScript is one of the scripts smolvm shares with Docker, for
// images run as containers: container/<name> in Assets.
func (s *Smolvm) containerScript(name string) ([]byte, error) {
	return fs.ReadFile(s.Assets, "container/"+name)
}

// KeyFiles are devvm's scripts for smolvm that go into base, one missing
// hashed as empty, and Ubuntu's pinned image.
func (s *Smolvm) KeyFiles() map[string][]byte {
	files := map[string][]byte{"ubuntu-image": []byte(ubuntu.ContainerRef)}
	for _, n := range []string{"prepare.sh", "shims.sh", "sshd.sh"} {
		files["container/"+n], _ = s.containerScript(n)
	}
	return files
}

// DiskDirs is devvm's smolvm folder, which holds the images' packs and, on
// Linux, smolvm's state with the machines; on macOS smolvm keeps machines
// under the user's home folder (backend.Storage).
func (s *Smolvm) DiskDirs() []string {
	dirs := []string{s.Dirs.Smolvm()}
	if s.OS != "linux" && s.Dirs.Home != "" {
		dirs = append(dirs, s.Dirs.Home)
	}
	return dirs
}

// Floors is the free space smolvm's steps want (backend.Storage),
// measured with statfs on macOS in the e2e sandbox. Most of each step's
// peak is gone when it ends: base took 1.9 GiB at its peak (0.3 kept),
// the docker layer 2.3 (0.4 kept), and making a docker VM 2.2 at its
// first start (8 MB kept). Shared tools builds took 0.55 GiB for node,
// pnpm and bun, and 5.6 GiB for the other seven.
func (s *Smolvm) Floors() backend.Floors {
	return backend.Floors{Base: 4 << 30, Image: 4 << 30, VM: 4 << 30, Tools: 4 << 30, PerTool: 512 << 20}
}

// ToolFormat is uncompressed EROFS (plain.sh), as smolvm's kernel mounts
// no other: its name changes with the script.
func (s *Smolvm) ToolFormat() string {
	plain, _ := s.script("plain.sh") // missing, BuildTools fails
	sum := sha256.Sum256(plain)
	return "plain-" + hex.EncodeToString(sum[:])[:12]
}

// record is what devvm set on one of its machines, which smolvm machine ls
// doesn't show: kept in the VM's backend folder (paths.VM.Record).
type record struct {
	Image     string   `json:"image"` // its image's key
	Project   string   `json:"project,omitempty"`
	AgentDirs []string `json:"agent_dirs,omitempty"`
	Tools     string   `json:"tools,omitempty"`
	ToolsAt   string   `json:"tools_at,omitempty"`
	// Forwarding is set when the machine was made forwarding an SSH
	// agent (--ssh-agent), which smolvm fixes then; AgentOff when devvm's
	// choice for it is none, so each start gives it no agent (agent.go).
	Forwarding bool `json:"forwarding,omitempty"`
	AgentOff   bool `json:"agent_off,omitempty"`
	// PortsAll is set when it forwards every port, else Listed are those
	// it forwards: through its forwarder (forward.go), with none of
	// smolvm's.
	PortsAll bool                `json:"ports_all,omitempty"`
	Listed   []backend.PortRange `json:"listed_ports,omitempty"`
	Env      map[string]string   `json:"env,omitempty"`
	// Mediated is set once the machine started through devvm's decider:
	// smolvm then requires one at every start, until devvm removes that.
	Mediated bool `json:"mediated,omitempty"`
	// Confined is the lifeline (agent.pid: its pid and start time) of the
	// machine's last start through devvm's decider; "" after any other.
	Confined string `json:"confined,omitempty"`
	// Started is the lifeline of the machine's last start by Start, with
	// the agent devvm chose (agent.go); "" while a start is under way.
	Started string `json:"started,omitempty"`
	// SSHPort is the host loopback port its sshd is forwarded from, set
	// while it's started with it (Confinement.SSH); 0 otherwise.
	SSHPort int `json:"ssh_port,omitempty"`
	// SSHUp is set once this start's sshd runs (services.sh done), and
	// cleared before each start.
	SSHUp bool `json:"ssh_up,omitempty"`
	// NoSSHD is set when its last start with ssh found no sshd in its
	// image: it runs without.
	NoSSHD bool `json:"no_sshd,omitempty"`
}

// proxyFile holds the proxy URL, password and all, of a machine started
// with network policy: Start writes it (mode 600), or removes it without.
// devvm's execs name it (machine exec --secret-file), so smolvm reads it
// and only its path is in smolvm's arguments. Not create's --secret-file:
// smolvm ignores that for a machine made from a pack.
func (s *Smolvm) proxyFile(vm string) string { return s.Dirs.VM(vm).ProxyURL() }

// setProxyFile writes the machine's proxy URL, or removes its file for "".
func (s *Smolvm) setProxyFile(vm, u string) error {
	if u == "" {
		if err := os.Remove(s.proxyFile(vm)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return state.WriteFile(s.proxyFile(vm), []byte(u))
}

// proxyArgs are machine exec's options that give the command the proxy
// variables (backend.GuestProxyVars) from the machine's proxy file, none
// when it has none.
func (s *Smolvm) proxyArgs(vm string) []string {
	f := s.proxyFile(vm)
	if fi, err := os.Stat(f); err != nil || fi.Size() == 0 { //nolint:gosec // devvm's own file, in the VM's folder: the hash its name ends with
		return nil
	}
	var args []string
	for _, k := range backend.GuestProxyVars() {
		args = append(args, "--secret-file", k+"="+f)
	}
	return args
}

func (s *Smolvm) readRecord(vm string) (record, error) {
	var r record
	b, err := os.ReadFile(s.Dirs.VM(vm).Record())
	if errors.Is(err, fs.ErrNotExist) {
		return r, nil
	} else if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (s *Smolvm) writeRecord(vm string, r record) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteFile(s.Dirs.VM(vm).Record(), append(b, '\n'))
}

// imageKey returns the key of an image's build machine, and whether it is
// one.
func imageKey(name string) (string, bool) {
	key, ok := strings.CutPrefix(name, ImagePrefix)
	return key, ok && image.IsKey(key)
}

// pack is an image's pack: smolvm's stub, beside its .smolmachine.
func (s *Smolvm) pack(key string) string { return filepath.Join(s.Dirs.SmolvmImage(key), "image") }

func (s *Smolvm) packFile(key string) string { return s.pack(key) + ".smolmachine" }

// stateDir is smolvm's own state on Linux hosts.
func (s *Smolvm) stateDir() string { return filepath.Join(s.Dirs.Smolvm(), "state") }

// List lists devvm's machines, without images' and the tools build's.
func (s *Smolvm) List(ctx context.Context) ([]backend.Instance, error) {
	ms, err := s.ours(ctx)
	if err != nil {
		return nil, err
	}
	var out []backend.Instance
	for _, m := range ms {
		if _, ok := imageKey(m.Name); ok || m.Name == ToolsVM {
			continue
		}
		in, err := s.instance(m)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, nil
}

// Inspect returns one of devvm's machines.
func (s *Smolvm) Inspect(ctx context.Context, vm string) (backend.Instance, error) {
	m, err := s.inspect(ctx, vm)
	if errors.Is(err, errForeign) {
		return backend.Instance{}, fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	} else if err != nil {
		return backend.Instance{}, err
	}
	return s.instance(m)
}

func (s *Smolvm) instance(m machine) (backend.Instance, error) {
	r, err := s.readRecord(m.Name)
	if err != nil {
		return backend.Instance{}, err
	}
	return backend.Instance{
		Name: m.Name, Status: m.status(), Size: m.size(),
		AgentDirs: slices.Clone(r.AgentDirs), AgentForwarding: r.Forwarding && !r.AgentOff, AgentFixed: !r.Forwarding,
		// smolvm forwards the agent from the machine's start, not
		// through ssh sessions.
		AgentWhileRunning: true,
	}, nil
}

// Stop stops one of devvm's machines; a stopped one is left as it is.
func (s *Smolvm) Stop(ctx context.Context, vm string) error {
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return notFound(vm, err)
	}
	if m.status() == backend.StatusStopped {
		return nil
	}
	return s.stop(ctx, vm)
}

// Delete deletes one of devvm's machines, stopping it first.
func (s *Smolvm) Delete(ctx context.Context, vm string) error {
	if _, err := s.inspect(ctx, vm); err != nil {
		return notFound(vm, err)
	}
	return s.remove(ctx, vm)
}

// notFound makes a machine that isn't devvm's one it doesn't have.
func notFound(vm string, err error) error {
	if errors.Is(err, errForeign) {
		return fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	}
	return err
}

// user is the VM's user: the host's name and uid.
func (s *Smolvm) user() (name, uid string, err error) {
	u, err := s.User()
	if err != nil {
		return "", "", err
	}
	return u.Username, u.Uid, nil
}

func home(name string) string { return "/home/" + name + ".guest" }

// loginEnv is what runs a command as the VM's user would get from logging
// in: smolvm's exec sets HOME alone, and reads no /etc/environment, which
// devvm's scripts write to (BASH_ENV).
const loginEnv = `set -a; [ -r /etc/environment ] && . /etc/environment; set +a; exec "$@"`

// userArgs are machine exec's arguments that run argv as the VM's user, in
// dir.
func (s *Smolvm) userArgs(vm, dir string, input, tty bool, argv []string) ([]string, error) {
	name, _, err := s.user()
	if err != nil {
		return nil, err
	}
	if dir == "" {
		dir = "/"
	}
	args := append([]string{"machine", "exec", "--name", vm}, s.proxyArgs(vm)...)
	if input {
		args = append(args, "-i")
	}
	if tty {
		args = append(args, "-t")
	}
	args = append(args, "-u", name, "-w", dir,
		"-e", "USER="+name, "-e", "LOGNAME="+name, "-e", "HOME="+home(name), "-e", "SHELL=/bin/bash",
		"--", "sh", "-c", loginEnv, "sh")
	return append(args, argv...), nil
}

// Exec runs argv in the VM as its user, in o.Dir. smolvm passes each
// argument as it is: no shell reads them.
func (s *Smolvm) Exec(ctx context.Context, vm string, argv []string, o backend.ExecOpts) error {
	args, err := s.userArgs(vm, o.Dir, o.Stdin != nil, false, argv)
	if err != nil {
		return err
	}
	cmd := s.smolvm(ctx, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.Stdin, o.Stdout, o.Stderr
	return s.Runner.Attach(cmd)
}

// Shell returns a login shell in dir as the VM's user, with a terminal when
// devvm has one.
func (s *Smolvm) Shell(ctx context.Context, vm, dir string) (backend.Command, error) {
	args, err := s.userArgs(vm, dir, true, s.Terminal(), []string{"bash", "-l"})
	if err != nil {
		return backend.Command{}, err
	}
	cmd := s.smolvm(ctx, args...)
	if cmd.Err != nil {
		return backend.Command{}, cmd.Err
	}
	return backend.Command{Path: cmd.Path, Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir}, nil
}

// stopped returns a machine that must be stopped to be changed, and its
// record.
func (s *Smolvm) stopped(ctx context.Context, vm string) (machine, record, error) {
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return machine{}, record{}, notFound(vm, err)
	}
	if m.status() != backend.StatusStopped {
		return machine{}, record{}, msg.VMNotStopped(vm)
	}
	r, err := s.readRecord(vm)
	return m, r, err
}

// SetSize changes a stopped VM's size where it differs. Disks only grow.
func (s *Smolvm) SetSize(ctx context.Context, vm string, sz backend.Size) error {
	if sz == (backend.Size{}) {
		return nil
	}
	m, _, err := s.stopped(ctx, vm)
	if err != nil {
		return err
	}
	cur := m.size()
	var opts []string
	if sz.CPUs > 0 && sz.CPUs != cur.CPUs {
		opts = append(opts, "--cpus", strconv.Itoa(sz.CPUs))
	}
	if sz.MemoryGiB > 0 && sz.MemoryGiB != cur.MemoryGiB {
		opts = append(opts, "--mem", mib(sz.MemoryGiB))
	}
	if sz.DiskGiB > 0 && sz.DiskGiB < cur.DiskGiB {
		return msg.DiskShrink(gibString(cur.DiskGiB), vm, gibString(sz.DiskGiB))
	}
	if sz.DiskGiB > cur.DiskGiB {
		opts = append(opts, "--storage", gibCeil(sz.DiskGiB))
	}
	return s.update(ctx, vm, opts...)
}

func gibString(g float64) string { return strconv.FormatFloat(g, 'g', -1, 64) }

// mib is GiB as smolvm's --mem takes it.
func mib(g float64) string { return strconv.Itoa(int(g * 1024)) }

// gibCeil is a disk size as smolvm's --storage takes it: whole GiB.
func gibCeil(g float64) string {
	n := int(g)
	if float64(n) < g {
		n++
	}
	return strconv.Itoa(n)
}

// listed is the ports a VM with ports p records as listed: none for
// every one.
func listed(p backend.Ports) []backend.PortRange {
	if p.All {
		return nil
	}
	return slices.Clone(p.Ranges)
}

// SetPorts changes which of a VM's ports its forwarder forwards (every
// one, its listed ones, or none): its record, which a stopped VM forwards
// from its next start, and a running one's forwarder, started again with
// them. Only devvm's record changes: smolvm forwards none of them. One
// started outside devvm since has no sshd of devvm's to forward through.
func (s *Smolvm) SetPorts(ctx context.Context, vm string, p backend.Ports, c backend.Confinement) error {
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return notFound(vm, err)
	}
	r, err := s.readRecord(vm)
	if err != nil {
		return err
	}
	if want := listed(p); r.PortsAll != p.All || !slices.Equal(r.Listed, want) {
		r.PortsAll, r.Listed = p.All, want
		if err := s.writeRecord(vm, r); err != nil {
			return err
		}
	}
	if m.status() != backend.StatusRunning {
		return nil
	}
	if cur, err := s.lifeline(ctx, vm); err != nil || !runsOn(cur, r.Started) {
		r.SSHUp = false
	}
	s.forward(ctx, vm, r, c)
	return nil
}

// volume is a -v value: host path, its path in the VM, and :ro unless
// writable. A path with a colon is refused: smolvm splits on colons.
func volume(host, at string, writable bool) (string, error) {
	for _, p := range []string{host, at} {
		if strings.Contains(p, ":") {
			return "", msg.SmolvmMountColon(p)
		}
	}
	v := host + ":" + at
	if !writable {
		v += ":ro"
	}
	return v, nil
}

// SetAgentDirs replaces a stopped VM's agent folder mounts.
func (s *Smolvm) SetAgentDirs(ctx context.Context, vm string, dirs []string) error {
	_, r, err := s.stopped(ctx, vm)
	if err != nil {
		return err
	}
	if slices.Equal(r.AgentDirs, dirs) {
		return nil
	}
	var opts []string
	for _, d := range r.AgentDirs {
		opts = append(opts, "--remove-volume", d+":"+d)
	}
	for _, d := range dirs {
		v, err := volume(d, d, false)
		if err != nil {
			return err
		}
		opts = append(opts, "-v", v)
	}
	if err := s.update(ctx, vm, opts...); err != nil {
		return err
	}
	r.AgentDirs = slices.Clone(dirs)
	return s.writeRecord(vm, r)
}

// ProxyURL is devvm's proxy as a machine reaches it, at the gateway; ""
// unless c.Net is NetProxy.
func (s *Smolvm) ProxyURL(c backend.Confinement) string {
	if c.Net != backend.NetProxy {
		return ""
	}
	return backend.ProxyURL(Gateway, c.ProxyPort, c.ProxyPassword)
}

// sortedEnv is env as KEY=value, sorted.
func sortedEnv(env map[string]string) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(env)) {
		out = append(out, k+"="+env[k])
	}
	return out
}
