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
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/sshconfig"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/vmname"
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
	OS       string           // the host's, as Go names it
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

// NewEnv returns the environment up runs in here, with devvm's own files
// (assets), devvm being self at version, on u. Its caller sets
// PickFeatures and PickParts.
func NewEnv(d paths.Dirs, assets fs.FS, self, version string, reg *backend.Registry, u UI) (*Env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if wd, err = filepath.EvalSymlinks(wd); err != nil {
		return nil, err
	}
	e := &Env{
		Dirs: d, Assets: assets, Registry: reg, Tools: tools.New(d, assets, version, u, reg.Host),
		Proxy: netpolicy.NewProxy(d, assets, self, secrets.NewStore(d, u.Warn), u),
		Dir:   wd, OS: runtime.GOOS, Version: version,
		Host: func() (Host, error) { return ThisHost(reg.Host) }, Exec: execShell, OpenURL: openURL, Environ: os.Environ(),
		Getenv: os.Getenv, HostTZ: hostTZ, GitConfig: gitConfig,
		SSHAdd: func(ctx context.Context, args ...string) int { return sshAdd(ctx, u.Stderr(), args...) },
		Info:   u.Info, Warn: u.Warn, Note: u.Note, Say: u.Say,
		Ask: u.Ask, Confirm: func(m msg.M) bool { return u.Confirm(m) }, Answer: u.Answer, Terminal: u.StdinIsTerminal,
		Interactive: func() bool { return u.StdinIsTerminal() && u.StdoutIsTerminal() }, CanPick: u.CanPick,
		Stdout: u.Stdout(), LogNote: u.LogNote, LogWriter: u.LogWriter, LoggedCmd: u.LoggedCmd,
		AddCleanup: u.AddCleanup, Interrupted: u.Interrupted,
		PortInUse: platform.PortInUse, PublicImage: from.NewLookup(d, u.Warn).Public, FreeSpace: reg.Host.FreeSpace,
	}
	e.Protector = func(dir string) *gitprotect.Protector { return gitprotect.New(dir, d.Home, e.Warn) }
	e.Links = func(st *state.VM) ([]GuestLink, error) { return guestLinks(d, st) }
	e.Agent = sshagent.New(d, e.mounts, u)
	return e, nil
}

// builder returns what builds images on b whose base starts from
// fromRef ("" for the backend's own).
func (e *Env) builder(b backend.Backend, fromRef string) *build.Env {
	return &build.Env{
		Dirs: e.Dirs, Assets: e.Assets, Backend: b, Review: e.Registry.Review, Tools: e.Tools, From: fromRef, Version: e.Version,
		Info: e.Info, Warn: e.Warn, Say: e.Say, Confirm: e.Confirm, Pick: e.PickFeatures,
		Terminal: e.Terminal, LogNote: e.LogNote, FreeSpace: e.FreeSpace,
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

// found is the folder's VM as devvm finds it.
type found struct {
	f      project.Folder
	st     *state.VM       // its state; one that doesn't exist for a new VM
	b      backend.Backend // its backend: the one its state records, else a new VM's
	exists bool            // the backend has it
	// sub is the folder devvm runs in when the VM is a parent folder's
	// (f.Dir), and so mounts it at the same path; "" when it's this
	// folder's own.
	sub string
	// repo is, for a parent folder's VM, the nearest folder from sub up,
	// below f.Dir, with a .git of its own; "" when none has one.
	repo string
}

// find finds the folder's VM, which needn't exist, with the backend that
// has it or would make it. When the folder has none, it's the nearest
// parent folder's VM, if one has one (lookup), said in an info line
// first. A folder with a repo of its own below the parent's (found.repo)
// never gets the parent's VM here: only up offers it.
func (e *Env) find(ctx context.Context) (*found, error) {
	t, parent, err := e.lookup(ctx, true)
	if err != nil {
		return nil, err
	}
	if parent != nil && parent.repo == "" {
		e.Info(msg.UsingParentVM(parent.f.Dir, parent.f.Name))
		return parent, nil
	}
	return t, nil
}

// lookup finds the folder's own VM, which needn't exist, as find does;
// with parents, when it has neither state nor a VM, also the VM of the
// nearest parent folder with state of its own (project.Parents, stopping
// before home and the root) when its backend has it and it was made for
// that folder (sameFolder), else nil.
// A folder's VM is found by its path's hash (project.Hash), which names
// its folder in devvm's data folder: one that exists has the name and
// the backend its state records, whichever backend is asked now. Only a
// new VM gets a name, fitting its backend's.
func (e *Env) lookup(ctx context.Context, parents bool) (*found, *found, error) {
	f, st, err := vmFolders{e}.state(e.Dir)
	if err != nil {
		return nil, nil, err
	}
	if !st.Exists() {
		if f, err = (vmFolders{e}).newName(e.Dir); err != nil {
			return nil, nil, err
		}
		if st, err = state.ReadVM(e.Dirs, f.Name); err != nil {
			return nil, nil, err
		}
	}
	// A folder with state of its own has a VM, even one its backend has
	// lost: never a parent's in its place.
	t, err := e.inspect(ctx, f, st)
	if err != nil || t.exists || st.Exists() || !parents {
		return t, nil, err
	}
	p, err := e.parentVM(ctx)
	if err != nil {
		return nil, nil, err
	}
	return t, p, nil
}

// state reads the state of dir's VM, by its path's hash, and returns the
// folder with the name it records ("" for none).
func (e vmFolders) state(dir string) (project.Folder, *state.VM, error) {
	if _, err := project.New(dir, 0); err != nil {
		if bad, ok := errors.AsType[*project.BadPathError](err); ok {
			return project.Folder{}, nil, msg.BadPath(bad.Dir)
		}
		return project.Folder{}, nil, err
	}
	st, err := state.ReadHash(e.Dirs, project.Hash(dir))
	if err == nil {
		err = checkState(st)
	}
	if err != nil {
		return project.Folder{}, nil, err
	}
	return project.Folder{Dir: dir, Name: st.Name()}, st, nil
}

// newName is the name a new VM for dir gets: fitting the names of the
// backend it would be made on.
func (e vmFolders) newName(dir string) (project.Folder, error) {
	first, err := e.Registry.For(new(state.VM))
	if err != nil {
		return project.Folder{}, err
	}
	nameMax := first.Backend.NameMax()
	f, err := project.New(dir, nameMax)
	if errors.Is(err, project.ErrNoRoom) {
		return project.Folder{}, msg.VMNameNoRoom(first.Backend.Name(), strconv.Itoa(nameMax))
	}
	return f, err
}

// parentVM finds the VM of the nearest parent folder with state of its own
// for lookup, else nil, with the nearest folder below it with a .git of
// its own (repo), the folder devvm runs in included. Only folders without
// state are passed over: the search stops at the first with state, which
// gives nil unless its backend has its VM and it was made for that folder.
// A VM in an outer folder can replace a nested folder through its mount,
// and devvm then never skips the nested folder's VM for the outer one's.
func (e *Env) parentVM(ctx context.Context) (*found, error) {
	repo := ""
	if hasGit(e.Dir) {
		repo = e.Dir
	}
	for _, dir := range project.Parents(e.Dir, e.Dirs.Home) {
		if _, err := project.New(dir, 0); err != nil {
			continue // a folder no VM can be made for
		}
		pf, pst, err := vmFolders{e}.state(dir)
		if err != nil {
			return nil, err
		}
		if !pst.Exists() {
			if repo == "" && hasGit(dir) {
				repo = dir
			}
			continue
		}
		if !e.sameFolder(pf, pst) {
			return nil, nil
		}
		p, err := e.inspect(ctx, pf, pst)
		if err != nil || !p.exists {
			return nil, err
		}
		p.sub, p.repo = e.Dir, repo
		return p, nil
	}
	return nil, nil
}

// hasGit reports whether dir has a .git of its own: a repo, or a
// worktree's or submodule's .git file.
func hasGit(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// inspect finds the VM for folder f, whose state is st, on the backend
// its state records, else a new VM's. One that exists gets devvm's
// connections carrying its SSH agent.
func (e *Env) inspect(ctx context.Context, f project.Folder, st *state.VM) (*found, error) {
	choice, err := e.Registry.For(st)
	if err != nil {
		return nil, err
	}
	b := choice.Backend
	if err := b.Check(ctx); err != nil {
		return nil, err
	}
	t := &found{f: f, st: st, b: b}
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
func (e *Env) findExisting(ctx context.Context) (*found, error) {
	t, err := e.find(ctx)
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
func (e *Env) existing(ctx context.Context) (*found, error) {
	t, err := e.findExisting(ctx)
	if err != nil {
		return nil, err
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Finished() {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// folderVM finds the folder's VM for a command that shows or changes it,
// which must exist (else noVM), be made for this folder and have finished
// being made.
func (e *Env) folderVM(ctx context.Context, noVM func(dir string) msg.M) (*found, error) {
	t, err := e.find(ctx)
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, noVM(t.f.Dir)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !t.st.Finished() {
		return nil, msg.VMUnfinished(t.f.Name)
	}
	return t, nil
}

// checkFolder refuses a VM made for another folder at the same path: one
// deleted and made again (another repo cloned there) never quietly gets
// the old one's disk.
func (e *Env) checkFolder(f project.Folder, st *state.VM) error {
	if !e.sameFolder(f, st) {
		return msg.VMOtherFolder(f.Name, st.Dir)
	}
	return nil
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

// sameFolder reports whether the VM was made for this folder: its path,
// and the folder itself. A VM without state has no folder to differ.
func (e *Env) sameFolder(f project.Folder, st *state.VM) bool {
	if !st.Exists() {
		return true
	}
	id, err := platform.FolderID(f.Dir)
	return err == nil && st.Dir == f.Dir && st.Folder == id
}

// FolderGone reports whether the project folder a VM's state records is
// gone: nothing is at its path, as when the folder was deleted or moved,
// or is on a disk that isn't connected. A folder made again at that path
// isn't gone, as sameFolder tells it apart, and neither is one whose path
// can't be read, or a VM whose state records none. A VM whose folder is
// gone is deleted only when asked for by name (Lifecycle.RmVM), never on
// its own.
func FolderGone(st *state.VM) bool {
	if st.Dir == "" {
		return false
	}
	_, err := os.Lstat(st.Dir)
	return errors.Is(err, fs.ErrNotExist)
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
	// The ports its backend forwards at each start (every port: an
	// emulation on some), which it got when they were set: refused now
	// unless they may still be used.
	p, err := ports.Of(st)
	if err != nil {
		return conf, err
	}
	forward, err := PortsBackend(p.Forward)
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
	if err := e.sshConfig().ForgetHostKeys(vm); err != nil {
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
		e.Proxy.Stop(vm)
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
	// Its listed ports less those taken, and its host key as its ssh
	// config pins it, for a backend's forwarder that reaches the VM's own
	// sshd.
	f := e.forwarding(b, st)
	conf.Free, conf.HostKey = f.Free, f.HostKey
	if conf.Net == backend.NetProxy {
		defer e.AddCleanup(undoStart(e, b, vm))()
	}
	// What its state asks of it, which the start makes it where it
	// differs: its size, its agents' folders on the host now, its agent
	// forwarding and its ports.
	agentDirs, _ := agentMounts(e.Dirs.Home, tools.Names(st))
	want := backend.Want{
		Size: sectionOf[Size](st, sizeSection).Backend(), AgentDirs: agentDirs,
		AgentForwarding: sshagent.Forwards(mode), Ports: forward,
	}
	if err := b.Start(ctx, vm, want, conf); err != nil {
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
	return conf, sshagent.RecordStart(ctx, st, mode, start)
}

// undoStart is a start's cleanup for a VM with network policy: an
// interrupt can leave it running with its backend's start still waiting
// for it, so without the record of its confined start (Confined), and
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
	}
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
	for _, f := range project.SetIDFiles(dir) {
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
