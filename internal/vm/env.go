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
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/hostsafety"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/sshconfig"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/vmname"
	"github.com/w3cj/devvm/internal/vmssh"
)

// Env is what the commands on a folder's VM run with, shared by Launch,
// Lifecycle, Settings and Net; its methods are what they share: starting
// the folder's VM, which the finder finds (finder.go), its size and ports,
// the image builder and the ssh config. NewEnv sets every field but Dir,
// which the finder reads when it's needed; tests change them.
type Env struct {
	Dirs     paths.Dirs
	Assets   fs.FS // devvm's own files (assets.FS)
	Registry *backend.Registry
	Tools    *tools.Env
	Proxy    *netpolicy.Proxy // VMs' network policy proxies
	Dir      string           // the folder devvm runs in, a physical path: "" until the finder needs it (finder.dir)
	// Folder is which VM the folder uses when a folder it's inside has
	// one (--here, --parent): resolveFolder's rule.
	Folder FolderRule
	OS     string // the host's, as Go names it
	// Version is devvm's version, which a new VM's state and each image's
	// record keep.
	Version string
	Host    func() (Host, error)
	// Protector returns the .git protection for a project folder.
	Protector func(dir string) *gitprotect.Protector
	// Exec runs the shell in devvm's place; it returns only on failure.
	Exec func(backend.Command) error
	// OpenURL opens a URL with macOS's open (devvm desktop's vnc://, in
	// Screen Sharing).
	OpenURL func(ctx context.Context, url string) error
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
	// Forwarders are VMs' port forwarders, as processes (forwards adds
	// the rest).
	Forwarders vmssh.Forwards
	// PublicImage pins an image a devcontainer.json names to the digest its
	// registry gives anyone (from.Lookup.Public).
	PublicImage func(ctx context.Context, ref string) (string, error)
	// FreeSpace returns the free space on the volume holding a path
	// (the registry's Host), checked before a new VM and each build; nil
	// checks none.
	FreeSpace func(path string) (uint64, error)
	// Links returns the links a VM's start and setup give it: its targets'
	// addresses and names, for its /etc/hosts and, with network policy,
	// its NO_PROXY (checkLinks checks them first); nil gives none.
	Links func(st *state.VM) ([]GuestLink, error)
	// What it prints and asks: Ask is [y/N], Confirm [Y/n]; PickFeatures
	// asks which features base gets (build.Env.Pick), PickParts shows the
	// tools picker (nil shows none); Terminal reports whether it can ask,
	// Interactive whether someone can also see the question (stdout's a
	// terminal too), CanPick whether the picker shows, which also needs no
	// --no-input, CanAsk whether a question with a fallback is asked (a
	// terminal, no --no-input; set by its caller); Answer asks for a value.
	Info, Warn, Note, Say                  func(msg.M)
	Ask, Confirm                           func(msg.M) bool
	Answer                                 func(msg.M) string
	PickFeatures                           func([]build.Feature) []build.Feature
	PickParts                              func(msg.M, []PickPart) []PickPart
	Terminal, Interactive, CanPick, CanAsk func() bool
	// AddInclude is --ssh-config: devvm's Include line goes in
	// ~/.ssh/config, when it's missing, without asking.
	AddInclude bool
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
	Ask(msg.M) bool
	Answer(msg.M) string
	StdoutIsTerminal() bool
	CanPick() bool
	Interrupted() bool
	Stdout() io.Writer
	Stderr() io.Writer
}

// NewEnv returns the environment up runs in, with devvm's own files
// (assets), devvm being self at version, on u, with the run's registry,
// shared tools and secrets store. The shared tools' downloads and builds,
// and --from's lookups, go through tl's company proxy, whose password
// its caller takes from store, the same secrets as the VMs' proxies'.
// Its caller sets PickFeatures and PickParts.
func NewEnv(d paths.Dirs, assets fs.FS, self, version string, reg *backend.Registry, tl *tools.Env, store *secrets.Store, u UI) *Env {
	e := &Env{
		Dirs: d, Assets: assets, Registry: reg, Tools: tl,
		Proxy: netpolicy.NewProxy(d, assets, self, store, u),
		OS:    runtime.GOOS, Version: version,
		Host: func() (Host, error) { return ThisHost(reg.Host) }, Exec: execShell, OpenURL: openURL, Environ: os.Environ(),
		Getenv: os.Getenv, HostTZ: hostTZ, GitConfig: gitConfig,
		SSHAdd: func(ctx context.Context, args ...string) int { return sshAdd(ctx, u.Stderr(), args...) },
		Info:   u.Info, Warn: u.Warn, Note: u.Note, Say: u.Say,
		Ask: u.Ask, Confirm: func(m msg.M) bool { return u.Confirm(m) }, Answer: u.Answer, Terminal: u.StdinIsTerminal,
		Interactive: func() bool { return u.StdinIsTerminal() && u.StdoutIsTerminal() }, CanPick: u.CanPick,
		Stdout: u.Stdout(), LogNote: u.LogNote, LogWriter: u.LogWriter, LoggedCmd: u.LoggedCmd,
		AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
		PortInUse: platform.PortInUse, Forwarders: vmssh.NewForwards(d, self), PublicImage: from.NewLookup(d, u.Warn, tl.Proxy).Public, FreeSpace: reg.Host.FreeSpace,
	}
	e.Protector = func(dir string) *gitprotect.Protector { return gitprotect.New(dir, d.Home, e.Warn) }
	e.Links = func(st *state.VM) ([]GuestLink, error) { return guestLinks(d, st) }
	e.Agent = sshagent.New(d, e.mounts, u)
	return e
}

// Builder returns what builds images on b whose base starts from
// fromRef ("" for the backend's own): the only one, which cli's
// commands take too.
func (e *Env) Builder(b backend.Backend, fromRef string) *build.Env {
	return &build.Env{
		Dirs: e.Dirs, Assets: e.Assets, Backend: b, Review: e.Registry.Review, Tools: e.Tools, From: fromRef, Version: e.Version,
		Info: e.Info, Warn: e.Warn, Say: e.Say, Confirm: e.Confirm, Pick: e.PickFeatures,
		Terminal: e.Terminal, LogNote: e.LogNote, LogWriter: e.LogWriter, FreeSpace: e.FreeSpace,
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

// openURL opens url with macOS's open.
func openURL(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "open", url).Run() //nolint:gosec // devvm's own vnc://127.0.0.1:PORT
}

// widen is asked, with e's prompts, before a command that gives the VM more
// access (cmd, as devvm network off) changes it: true for the folder's own
// VM. A parent folder's VM, found from a subfolder, changes only after
// asking, [y/N], and never without a terminal.
func (t *found) widen(e *Env, cmd string) (bool, error) {
	if t.sub == "" {
		return true, nil
	}
	if !e.Terminal() {
		return false, msg.WidenParentVMRefused(cmd, t.f.Dir, t.f.Name)
	}
	return e.Ask(msg.WidenParentVM(cmd, t.f.Dir, t.f.Name)), nil
}

// FolderState is what's at the path of the project folder a VM's state
// records.
type FolderState string

// The folder states. A VM whose folder is gone or not connected is
// deleted only when asked for by name (Lifecycle.RmVM), never on its own.
const (
	// FolderThere: something is at the path, or it can't be read, or the
	// state records no folder. A folder made again there is there too:
	// sameFolder tells it apart.
	FolderThere FolderState = "there"
	// FolderGone: nothing is at the path, and the nearest folder above it
	// that exists is on the device the folder was on (deleted or moved),
	// or the state records no device to compare.
	FolderGone FolderState = "gone"
	// FolderNotConnected: nothing is at the path, and the nearest folder
	// above it is on another device: the folder was on a disk or share
	// that isn't connected, or moved to another disk, then deleted.
	FolderNotConnected FolderState = "not_connected"
)

// FolderOf is the state of the project folder st records.
func FolderOf(st *state.VM) FolderState { return folderOf(st, platform.DeviceOf) }

// folderOf is FolderOf with device, which returns the device of what's at
// a path (platform.DeviceOf), a test seam.
func folderOf(st *state.VM, device func(string) (string, error)) FolderState {
	if st.Dir == "" {
		return FolderThere
	}
	if _, err := os.Lstat(st.Dir); !errors.Is(err, fs.ErrNotExist) {
		return FolderThere
	}
	was := platform.FolderDevice(st.Folder)
	for dir := filepath.Dir(st.Dir); was != ""; dir = filepath.Dir(dir) {
		if dev, err := device(dir); err == nil {
			if dev != was {
				return FolderNotConnected
			}
			break
		} else if !errors.Is(err, fs.ErrNotExist) || dir == filepath.Dir(dir) {
			break
		}
	}
	return FolderGone
}

// recordedVMs returns the project VMs that have state, as their states
// record them.
func recordedVMs(d paths.Dirs) ([]project.Recorded, error) {
	names, err := state.VMNames(d)
	if err != nil {
		return nil, err
	}
	var out []project.Recorded
	for _, n := range names {
		st, err := state.ReadVM(d, n)
		if err != nil {
			return nil, err
		}
		out = append(out, project.Recorded{Name: n, Alias: st.Alias, Dir: st.Dir})
	}
	return out, nil
}

// start starts a stopped VM confined as conf says, and returns the
// confinement it got. With network policy its proxy starts first, and the
// backend gets the proxy's confinement; a start that fails stops the proxy
// again. Without it, a proxy left from when it had it stops. Its host keys
// are read again once it runs: a VM whose backend config changed can get
// new ones when it boots. A VM whose creation didn't finish never starts.
func (e *Env) start(ctx context.Context, b backend.Backend, st *state.VM, conf backend.Confinement) (backend.Confinement, error) {
	vm := st.Name()
	if !st.Finished() {
		return conf, msg.VMUnfinished(vm)
	}
	// The ports devvm's forwarder forwards from its backend's ssh at each
	// start (an emulation on some), which it got when they were set:
	// refused now unless they may still be used.
	p, err := ports.Of(st)
	if err != nil {
		return conf, err
	}
	forward, err := ports.Forwarding(p.Forward)
	if err != nil {
		return conf, err
	}
	for _, f := range forward.Features() {
		if !e.Registry.Allowed(b, f) {
			return conf, e.Registry.Require(b, vm, f)
		}
	}
	// So does one whose backend emulates turning its SSH agent forwarding
	// on or off, made ready for it.
	if a, err := sshagent.Of(st); err != nil {
		return conf, err
	} else if a.Made.Toggle && !e.Registry.Allowed(b, backend.FeatureAgentToggle) {
		return conf, e.Registry.Require(b, vm, backend.FeatureAgentToggle)
	}
	if err := vmssh.ForgetHostKeys(e.Dirs, vm); err != nil {
		return conf, err
	}
	// A link session of its last start's, which would end on its own: the
	// launch starts one for this start.
	linkRuns{e}.stop(ctx, vm)
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
		// It gets the shell's proxy settings alone: config.toml's
		// password never goes into a VM.
		if conf.Net == backend.NetDirect && e.Proxy.Company().ConfigOnly() {
			e.Warn(msg.ProxyNeedsPolicy(e.Dirs.ConfigFile(), vm))
		}
		if err := e.Proxy.Unconfined(ctx, vm); err != nil {
			return conf, err
		}
		// Its ssh port, as the proxy's confinement has it: recorded, so
		// other VMs' forwarders never take it before its backend binds it.
		sp, err := e.Proxy.Ports().SSH(ctx, st, false)
		if err != nil {
			return conf, err
		}
		conf.SSHPort = sp
	} else {
		p, err := e.Proxy.Prepare(ctx, b, st, false)
		if err != nil {
			return conf, err
		}
		// The proxy's confinement, with the agent chosen above; its links
		// checked first.
		p.AgentSocket, p.NoAgent = conf.AgentSocket, conf.NoAgent
		if err := (syncer{e}).checkStartLinks(st); err != nil {
			e.Proxy.Stop(vm)
			return conf, err
		}
		conf = p
	}
	// Tools pointed at the CAs devvm adds, with network policy or
	// without, whether there are any or not.
	conf.Env = trust.GuestEnv(conf.Env)
	// ssh for editors, where the backend has it (an emulation once it's
	// reviewed): a VM without it starts all the same.
	conf.SSH = e.Registry.Require(b, vm, backend.FeatureSSH) == nil
	if conf.Net == backend.NetProxy {
		defer e.AddCleanup(undoStart(e, b, vm))()
	}
	// What its state asks of it, which the start makes it where it
	// differs: its size, its agents' folders on the host now, its agent
	// forwarding.
	agentDirs, _ := agentMounts(e.Dirs.Home, tools.Names(st))
	want := backend.Want{
		Size: sectionOf[Size](st, sizeSection).Backend(), AgentDirs: agentDirs,
		AgentForwarding: sshagent.Forwards(mode),
	}
	if err := b.Start(ctx, vm, want, conf); err != nil {
		if conf.Net == backend.NetProxy {
			e.Proxy.Stop(vm)
		}
		return conf, err
	}
	// Its ports, once it runs: its state's, through devvm's forwarder.
	e.forwards().Start(ctx, b, st)
	// The agent it started with, until its next start: its ssh config
	// entry's and devvm's shell's. One whose start can't be told apart
	// gets none recorded, and follows its choice.
	run, err := b.Running(ctx, vm)
	if err != nil {
		run.ID = ""
	}
	return conf, sshagent.RecordStart(ctx, st, mode, run.ID)
}

// undoStart is a start's cleanup for a VM with network policy: an
// interrupt can leave it running with its backend's start still waiting
// for it, so without the record of its confined start (Run.Confined), and
// it stops the VM and its proxy. The next launch starts both again,
// confined.
func undoStart(e *Env, b backend.Backend, vm string) func(context.Context) {
	return func(ctx context.Context) {
		if !e.Interrupted() {
			return
		}
		e.Info(msg.InterruptedStart(vm))
		if err := b.Stop(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			e.Warn(msg.StopFailed(vm))
		}
		e.Proxy.Stop(vm)
		e.Forwarders.Stop(vm)
	}
}

// runningAgent is the SSH agent choice a running VM runs with: its
// current start's (sshagent.Running).
func runningAgent(ctx context.Context, b backend.Backend, st *state.VM) (string, error) {
	run, err := b.Running(ctx, st.Name())
	if err != nil {
		return "", err
	}
	return sshagent.Running(st, run.ID), nil
}

// ensureRunning makes sure the VM runs, for a command that runs in it
// without a shell (an export, rm's check of a --clone VM): it goes
// through what a launch's start does (startChecked), .git locked, network
// policy's self-check and the refusals included.
func (e *Env) ensureRunning(ctx context.Context, t *found) error {
	_, err := Launch{e}.startChecked(ctx, t)
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
			if !vmname.IsVM(in.Name) {
				continue
			}
			st, err := state.ReadVM(e.Dirs, in.Name)
			if err != nil {
				return nil, err
			}
			if st.Dir != "" && !isClone(st) {
				ms = append(ms, sshagent.Mount{VM: in.Name, Dir: st.Dir})
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
	c.AddInclude = e.AddInclude
	return c
}

// checkSetID names set-id files in the folder on Linux hosts, where they
// reach the host (macOS's virtiofs drops the bits): those running as root
// apart.
func (e *Env) checkSetID(dir string) {
	if e.OS != "linux" {
		return
	}
	var root, other []string
	for _, f := range hostsafety.SetIDFiles(dir) {
		if f.Root {
			root = append(root, f.Path)
		} else {
			other = append(other, f.Path)
		}
	}
	if len(root) > 0 {
		e.Warn(msg.SetidRootFiles(strings.Join(root, " ")))
	}
	if len(other) > 0 {
		e.Warn(msg.SetidFiles(strings.Join(other, " ")))
	}
}
