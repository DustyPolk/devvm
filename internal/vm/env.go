// Package vm runs project VMs. Env is what they run with; Launch opens a
// shell in a folder's VM, making it first if need be; Lifecycle stops,
// adopts, deletes and exports it; Settings shows and changes its size,
// ports, SSH agent forwarding and .git protection; Net its network policy,
// network log, allowlist and secrets.
package vm

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/sshconfig"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Keys of a VM's state that up writes, besides state's own.
const (
	KeyNet      = netpolicy.KeyNet // network policy: on or off
	KeyLibc     = "libc"           // the C library its base had, which its tools are built for
	KeyTools    = "tools"          // its shared tool images, space-separated
	KeyCPUs     = "cpus"           // the size asked for; unset for the image's
	KeyMemory   = "memory"         // GiB
	KeyDisk     = "disk"           // GiB
	KeyPorts    = "ports"          // which ports it forwards (ParsePorts); unset for all
	KeyPortsSet = "ports_set"      // what its backend config forwards; unset for all
	// KeyAgentToggle is set when its backend emulates turning its SSH agent
	// forwarding on or off (made ready for it), which its starts rely on.
	KeyAgentToggle = "agent_toggle"
	// KeyDockerLayer is the runtime it runs the docker layer under, on a
	// backend that makes VMs for it (backend.DockerLayerVM): sysbox; unset
	// for a VM that runs it as any VM, or doesn't have it. A VM with it set
	// gets no network policy.
	KeyDockerLayer = "docker_layer"
)

// Env is what the commands on a folder's VM run with, shared by Launch,
// Lifecycle, Settings and Net; its methods are what they share: finding
// the folder's VM, starting it, its size and ports, the image builder and
// the ssh config. NewEnv sets every field; tests change them.
type Env struct {
	Dirs     paths.Dirs
	Assets   fs.FS // devvm's own files (assets.FS)
	Registry *backend.Registry
	Tools    *tools.Env
	Proxy    *netpolicy.Proxy // VMs' network policy proxies
	Dir      string           // the folder devvm runs in, a physical path
	Home     string
	OS       string // the host's, as Go names it
	Host     func() (Host, error)
	// Protector returns the .git protection for a project folder.
	Protector func(dir string) *gitprotect.Protector
	// Exec runs the shell in devvm's place; it returns only on failure.
	Exec func(backend.Command) error
	// Environ is the environment devvm started with, the SSH agent
	// included: the editor's, which devvm's own connections don't change.
	Environ []string
	// What a VM's setup sends of the host's settings: the environment
	// (TZ, LANG, LC_ALL), the timezone, and git's settings in a folder.
	Getenv    func(string) string
	HostTZ    func() string
	GitConfig func(ctx context.Context, dir, key string) string
	// SSHAdd runs ssh-add with args, on the user's own agent, and returns
	// its exit code.
	SSHAdd func(ctx context.Context, args ...string) int
	// Agent is devvm's own SSH agent.
	Agent *sshagent.Agent
	// PortInUse reports whether a program on the host has a port, on any
	// address (platform.PortInUse): a listed port that isn't forwarded.
	PortInUse func(ctx context.Context, port int) bool
	// PublicImage pins an image a devcontainer.json names to the digest its
	// registry gives anyone (from.Lookup.Public).
	PublicImage func(ctx context.Context, ref string) (string, error)
	// What it prints and asks: Ask is [y/N], Confirm [Y/n]; PickFeatures
	// asks which features base gets (build.Env.Pick); Terminal reports
	// whether it can ask.
	Info, Warn, Note, Say func(msg.M)
	Ask, Confirm          func(msg.M) bool
	PickFeatures          func([]build.Feature) []build.Feature
	Terminal              func() bool
	// Stdout is where data output goes.
	Stdout io.Writer
	// The run log: LogNote says where it is, once a run; LogWriter is
	// where the output of a backend's Exec goes; LoggedCmd runs a command
	// into it.
	LogNote   func()
	LogWriter func() io.Writer
	LoggedCmd func(ctx context.Context, step msg.M, cmd *exec.Cmd) error
	// AddCleanup adds to the run's cleanup stack; Interrupted reports
	// whether the run was interrupted.
	AddCleanup  func(f func(context.Context)) (remove func())
	Interrupted func() bool
}

// UI is what up and the commands on a VM need of devvm's UI: where they
// print and ask, the run log, the cleanup stack, and data output.
type UI interface {
	tools.UI
	netpolicy.UI
	sshagent.UI
	Say(msg.M)
	Interrupted() bool
	Stdout() io.Writer
	Stderr() io.Writer
}

// NewEnv returns the environment up runs in here, with devvm's own files
// (assets), devvm being self, on u. Its caller sets PickFeatures.
func NewEnv(d paths.Dirs, assets fs.FS, self string, reg *backend.Registry, u UI) (*Env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if wd, err = filepath.EvalSymlinks(wd); err != nil {
		return nil, err
	}
	home := os.Getenv("HOME")
	e := &Env{
		Dirs: d, Assets: assets, Registry: reg, Tools: tools.New(d, assets, u),
		Proxy: netpolicy.NewProxy(d, assets, self, secrets.NewStore(d, home, u.Warn), u),
		Dir:   wd, Home: home, OS: runtime.GOOS,
		Host: ThisHost, Exec: execShell, Environ: os.Environ(),
		Getenv: os.Getenv, HostTZ: hostTZ, GitConfig: gitConfig,
		SSHAdd: func(ctx context.Context, args ...string) int { return sshAdd(ctx, u.Stderr(), args...) },
		Info:   u.Info, Warn: u.Warn, Note: u.Note, Say: u.Say,
		Ask: u.Ask, Confirm: func(m msg.M) bool { return u.Confirm(m) }, Terminal: u.StdinIsTerminal,
		Stdout: u.Stdout(), LogNote: u.LogNote, LogWriter: u.LogWriter, LoggedCmd: u.LoggedCmd,
		AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
		PortInUse: platform.PortInUse, PublicImage: from.NewLookup(d, u.Warn).Public,
	}
	e.Protector = func(dir string) *gitprotect.Protector { return gitprotect.New(dir, e.Warn) }
	e.Agent = sshagent.New(d, e.mounts, u)
	return e, nil
}

// builder returns what builds images on b whose base starts from
// fromRef ("" for the backend's own).
func (e *Env) builder(b backend.Backend, fromRef string) *build.Env {
	return &build.Env{
		Dirs: e.Dirs, Assets: e.Assets, Backend: b, Review: e.Registry.Review, Tools: e.Tools, From: fromRef,
		Info: e.Info, Warn: e.Warn, Say: e.Say, Confirm: e.Confirm, Pick: e.PickFeatures,
		Terminal: e.Terminal, LogNote: e.LogNote,
	}
}

// execShell runs cmd in place of devvm, in its folder.
func execShell(cmd backend.Command) error {
	if cmd.Dir != "" {
		if err := os.Chdir(cmd.Dir); err != nil {
			return err
		}
	}
	return platform.Exec(cmd.Path, cmd.Args, cmd.Env)
}

// found is the folder's VM as devvm finds it.
type found struct {
	f      project.Folder
	st     *state.VM       // its state; one that doesn't exist for a new VM
	b      backend.Backend // its backend: the one its state records, else a new VM's
	exists bool            // the backend has it
	// flagIgnored is set when --backend named another backend than the
	// VM's.
	flagIgnored bool
}

// find finds the folder's VM, which needn't exist, with the backend that
// has it or would make it (flag is --backend). The VM's name fits a new
// VM's backend; a VM that exists keeps the backend its state records.
func (e *Env) find(ctx context.Context, flag string) (*found, error) {
	first, err := e.Registry.For(e.Dirs, new(state.VM), flag)
	if err != nil {
		return nil, err
	}
	f, err := project.New(e.Dir, first.Backend.NameMax())
	if bad, ok := errors.AsType[*project.BadPathError](err); ok {
		return nil, msg.BadPath(msg.Shown(bad.Dir))
	} else if errors.Is(err, project.ErrNoRoom) {
		return nil, msg.VMNameNoRoom(first.Backend.Name(), strconv.Itoa(first.Backend.NameMax()))
	} else if err != nil {
		return nil, err
	}
	st, err := state.ReadVM(e.Dirs, f.Name)
	if err != nil {
		return nil, err
	}
	choice, err := e.Registry.For(e.Dirs, st, flag)
	if err != nil {
		return nil, err
	}
	b := choice.Backend
	if err := b.Check(ctx); err != nil {
		return nil, err
	}
	t := &found{f: f, st: st, b: b, flagIgnored: choice.FlagIgnored}
	switch in, err := b.Inspect(ctx, f.Name); {
	case err == nil:
		t.exists = true
		mode := sshagent.Mode(st)
		if in.Status == backend.StatusRunning {
			if mode, err = runningAgent(ctx, b, st); err != nil {
				return nil, err
			}
		}
		e.useAgent(mode)
	case !errors.Is(err, backend.ErrNotFound):
		return nil, err
	}
	return t, nil
}

// findExisting finds the folder's VM, which must exist.
func (e *Env) findExisting(ctx context.Context, flag string) (*found, error) {
	t, err := e.find(ctx, flag)
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, msg.NoVM(t.f.Dir)
	}
	return t, nil
}

// existing finds the folder's VM, which must exist, be made for this
// folder and have finished being made.
func (e *Env) existing(ctx context.Context, flag string) (*found, error) {
	t, err := e.findExisting(ctx, flag)
	if err != nil {
		return nil, err
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Exists() || t.st.Get(KeyNet) == "" {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// folderVM finds the folder's VM for a command that shows or changes it,
// which must exist (else noVM), be made for this folder and have finished
// being made.
func (e *Env) folderVM(ctx context.Context, noVM func(dir string) msg.M) (*found, error) {
	t, err := e.find(ctx, "")
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, noVM(t.f.Dir)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Exists() || t.st.Get(KeyNet) == "" {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// checkFolder refuses a VM made for another folder at the same path: one
// deleted and made again (another repo cloned there) never quietly gets
// the old one's disk.
func (e *Env) checkFolder(f project.Folder, st *state.VM) error {
	if !e.sameFolder(f, st) {
		return msg.VMOtherFolder(f.Name, st.Get(state.KeyDir))
	}
	return nil
}

// sameFolder reports whether the VM was made for this folder: its path,
// and the folder itself. A VM without state has no folder to differ.
func (e *Env) sameFolder(f project.Folder, st *state.VM) bool {
	if !st.Exists() {
		return true
	}
	id, err := platform.FolderID(f.Dir)
	return err == nil && st.Get(state.KeyDir) == f.Dir && st.Get(state.KeyFolder) == id
}

// start starts a stopped VM confined as conf says, and returns the
// confinement it got. With network policy its proxy starts first, and the
// backend gets the proxy's confinement; a start that fails stops the proxy
// again. Without it, a proxy left from when it had it stops. Its host keys
// are read again once it runs: a VM whose backend config changed can get
// new ones when it boots. A VM without a network choice never starts: its
// creation didn't finish, and it would start without network policy.
func (e *Env) start(ctx context.Context, b backend.Backend, st *state.VM, conf backend.Confinement) (backend.Confinement, error) {
	vm := st.Name()
	if st.Get(KeyNet) == "" {
		return conf, msg.VMUnfinished(vm)
	}
	// The ports its backend forwards at each start (every port: an
	// emulation on some), which it got when they were set: refused now
	// unless they may still be used.
	for _, f := range PortsBackend(orAll(st.Get(KeyPortsSet))).Features() {
		if !e.Registry.Allowed(b, f) {
			return conf, e.Registry.Require(b, vm, f)
		}
	}
	// So does one whose backend emulates turning its SSH agent forwarding
	// on or off, made ready for it.
	if st.Get(KeyAgentToggle) != "" && !e.Registry.Allowed(b, backend.FeatureAgentToggle) {
		return conf, e.Registry.Require(b, vm, backend.FeatureAgentToggle)
	}
	if err := e.sshConfig().ForgetHostKeys(vm); err != nil {
		return conf, err
	}
	// The VM's backend forwards the agent it gets when it starts: devvm's
	// own, with the keys it lacks loaded, for a VM that uses it; none for
	// one that uses none.
	mode := sshagent.Mode(st)
	conf.NoAgent = !sshagent.Forwards(mode)
	if mode == sshagent.Devvm {
		if err := e.Agent.Ensure(ctx); err != nil {
			return conf, err
		}
		conf.AgentSocket = e.Agent.Socket()
	}
	// devvm's own connections carry the agent of the start they're on.
	e.useAgent(mode)
	if conf.Net != backend.NetProxy {
		e.Proxy.Stop(vm)
	} else {
		p, err := e.Proxy.Prepare(ctx, b, st, false)
		if err != nil {
			return conf, err
		}
		// The proxy's confinement, with the agent chosen above.
		p.AgentSocket, p.NoAgent = conf.AgentSocket, conf.NoAgent
		conf = p
	}
	// ssh for editors, where the backend has it (an emulation once it's
	// reviewed): a VM without it starts all the same.
	conf.SSH = e.Registry.Require(b, vm, backend.FeatureSSH) == nil
	conf.Ports = e.startPorts(ctx, st)
	if err := b.Start(ctx, vm, conf); err != nil {
		if conf.Net == backend.NetProxy {
			e.Proxy.Stop(vm)
		}
		return conf, err
	}
	// The agent it started with, until its next start: its ssh config
	// entry's and devvm's shell's. One whose start can't be told apart
	// gets none recorded, and follows its choice.
	start, err := b.StartID(ctx, vm)
	if err != nil {
		start = ""
	}
	return conf, sshagent.RecordStart(st, mode, start)
}

// runningAgent is the SSH agent choice a running VM runs with: its
// current start's (sshagent.Running).
func runningAgent(ctx context.Context, b backend.Backend, st *state.VM) (string, error) {
	start, err := b.StartID(ctx, st.Name())
	if err != nil {
		return "", err
	}
	return sshagent.Running(st, start), nil
}

// checkAgentStart stops a running VM that may forward an SSH agent devvm
// didn't choose for it, its backend says (one started outside devvm, on a
// backend whose starts name the agent), unless it uses the user's anyway.
// The stop isn't cancelled with ctx: an interrupt never leaves it running.
func (e *Env) checkAgentStart(ctx context.Context, b backend.Backend, st *state.VM) error {
	vm := st.Name()
	if sshagent.Mode(st) == sshagent.On {
		return nil
	}
	unchosen, err := backend.AgentUnchosen(ctx, b, vm)
	if err != nil || !unchosen {
		return err
	}
	if err := b.Stop(context.WithoutCancel(ctx), vm); err != nil {
		return err
	}
	e.Proxy.Stop(vm)
	return msg.AgentUnchosenStop(vm)
}

// ensureRunning starts the VM if it's stopped, confined as its state says;
// a running one is checked for an agent devvm didn't choose.
func (e *Env) ensureRunning(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if in.Status == backend.StatusRunning {
		return e.checkAgentStart(ctx, b, t.st)
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if netpolicy.On(t.st) {
		conf.Net = backend.NetProxy
	}
	if err := e.Registry.Require(b, vm, conf.Features()...); err != nil {
		return err
	}
	e.Info(msg.Starting(vm))
	_, err = e.start(ctx, b, t.st, conf)
	return err
}

// useAgent makes the SSH agent devvm's own connections to the folder's VM
// carry the one mode says: devvm's, the user's (the one devvm started
// with), or none. Its backend's connections to it (Lima's share one) and
// the shell's are among them.
func (e *Env) useAgent(mode string) {
	user, ok := "", false
	for _, kv := range e.Environ {
		if v, found := strings.CutPrefix(kv, "SSH_AUTH_SOCK="); found {
			user, ok = v, true
		}
	}
	switch {
	case mode == sshagent.Devvm:
		_ = os.Setenv("SSH_AUTH_SOCK", e.Agent.Socket())
	case mode == sshagent.On && ok:
		_ = os.Setenv("SSH_AUTH_SOCK", user)
	default:
		_ = os.Unsetenv("SSH_AUTH_SOCK")
	}
}

// mounts returns the host folders project VMs mount, as their state and
// backends have them: the project folder (not a --clone VM's), agent
// folders, the tools share. Only backends some VM's state names are
// asked; one that can't list runs no VMs.
func (e *Env) mounts(ctx context.Context) ([]sshagent.Mount, error) {
	bs, err := e.Registry.Used(e.Dirs)
	if err != nil {
		return nil, err
	}
	var ms []sshagent.Mount
	for _, b := range bs {
		ins, err := b.List(ctx)
		if err != nil {
			continue
		}
		for _, in := range ins {
			if !project.IsVM(in.Name) {
				continue
			}
			st, err := state.ReadVM(e.Dirs, in.Name)
			if err != nil {
				return nil, err
			}
			if dir := st.Get(state.KeyDir); dir != "" && !isClone(st) {
				ms = append(ms, sshagent.Mount{VM: in.Name, Dir: dir})
			}
			for _, d := range append(slices.Clone(in.AgentDirs), e.Dirs.Tools()) {
				ms = append(ms, sshagent.Mount{VM: in.Name, Dir: d})
			}
		}
	}
	return ms, nil
}

// sshConfig is the ssh config for every backend's VMs.
func (e *Env) sshConfig() *sshconfig.Config {
	c := sshconfig.New(e.Dirs, e.Registry)
	c.Info, c.Warn, c.Say, c.Confirm, c.Terminal = e.Info, e.Warn, e.Say, e.Confirm, e.Terminal
	return c
}

// checkSetID names set-id files in the folder on Linux hosts, where they
// reach the host (macOS's virtiofs drops the bits): those running as root
// (root in a Docker VM writes as the host's root) apart.
func (e *Env) checkSetID(dir string) {
	if e.OS != "linux" {
		return
	}
	var root, other []string
	for _, f := range project.SetIDFiles(dir) {
		if f.Root {
			root = append(root, msg.Shown(f.Path))
		} else {
			other = append(other, msg.Shown(f.Path))
		}
	}
	if len(root) > 0 {
		e.Warn(msg.SetidRootFiles(strings.Join(root, " ")))
	}
	if len(other) > 0 {
		e.Warn(msg.SetidFiles(strings.Join(other, " ")))
	}
}
