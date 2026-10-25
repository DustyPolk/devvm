// Package vm runs project VMs: up, which makes a folder's VM from its image
// and opens a shell in it, and the sizes and ports VMs get.
package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/image"
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
	"github.com/w3cj/devvm/internal/ui"
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
)

// Options is what up was asked for.
type Options struct {
	Image           string // the image a new VM gets, as given; "" to detect it
	Backend         string // --backend: a new VM's backend
	NetworkPolicy   bool   // --network-policy
	NoNetworkPolicy bool   // --no-network-policy
	Size            Size   // --cpus, --memory, --disk as given
	Ports           string // --ports as given
	UnsafeFolder    bool   // --unsafe-folder
	NoGitProtect    bool   // --no-git-protect
	Clone           bool   // --clone
	SSHAgent        bool   // --ssh-agent: a new VM forwards the user's SSH agent
	From            string // --from, or its default: the container image a new VM's base starts from
}

// Env is what up runs with. FromEnv sets every field; tests change them.
type Env struct {
	Dirs     paths.Dirs
	Root     string // devvm's own files (paths.Root)
	Registry *backend.Registry
	Tools    *tools.Tools
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
}

// NewEnv returns the environment up runs in here, devvm being self.
func NewEnv(d paths.Dirs, root, self string, reg *backend.Registry) (*Env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if wd, err = filepath.EvalSymlinks(wd); err != nil {
		return nil, err
	}
	home := os.Getenv("HOME")
	e := &Env{
		Dirs: d, Root: root, Registry: reg, Tools: tools.New(d, root),
		Proxy: netpolicy.NewProxy(d, root, self, secrets.NewStore(d, home)),
		Dir:   wd, Home: home, OS: runtime.GOOS,
		Host: ThisHost, Protector: gitprotect.New, Exec: execShell, Environ: os.Environ(),
		Getenv: os.Getenv, HostTZ: hostTZ, GitConfig: gitConfig, SSHAdd: sshAdd,
	}
	e.Agent = sshagent.New(d, e.mounts)
	return e, nil
}

// builder returns the builder of images on b whose base starts from
// fromRef ("" for the backend's own).
func (e *Env) builder(b backend.Backend, fromRef string) *build.Builder {
	return &build.Builder{Dirs: e.Dirs, Root: e.Root, Backend: b, Tools: e.Tools, From: fromRef}
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

// Up opens a shell in the folder's VM, making it first: checked, from
// its image (built if it isn't), with only the folder mounted, .git
// protected, started; with --clone, nothing mounted but its own clone of
// the repo (seedClone). A VM that exists is opened as it is, with what's
// changed since checked (open). On success it doesn't return: the shell
// takes devvm's place.
func (e *Env) Up(ctx context.Context, o Options) error {
	if err := from.Check(o.From); err != nil {
		return err
	}
	size, err := o.Size.Parse()
	if err != nil {
		return err
	}
	ports := ""
	if o.Ports != "" {
		if ports, err = ParsePorts(o.Ports); err != nil {
			return err
		}
	}
	t, err := e.find(ctx, o.Backend)
	if err != nil {
		return err
	}
	if !t.exists {
		return e.create(ctx, o, t.f, t.b, size, ports)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return err
	}
	return e.open(ctx, o, t)
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
	switch _, err := b.Inspect(ctx, f.Name); {
	case err == nil:
		t.exists = true
		e.useAgent(sshagent.Mode(st))
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

// create makes the folder's VM and opens a shell in it.
func (e *Env) create(ctx context.Context, o Options, f project.Folder, b backend.Backend, size Size, ports string) error {
	vm := f.Name
	if o.Clone {
		if err := checkCloneRepo(ctx, f.Dir); err != nil {
			return err
		}
	} else if err := e.checkUnsafe(f.Dir, o.UnsafeFolder); err != nil {
		return err
	}
	// Defaults are checked again: the files may be edited by hand. All
	// before the build, which can take minutes.
	def, err := e.defaultSize()
	if err != nil {
		return err
	}
	size = size.Or(def)
	host, err := e.Host()
	if err != nil {
		return err
	}
	if err := size.Check(host, BaseDisk(e.Root), vm); err != nil {
		return err
	}
	if ports == "" {
		if ports, err = e.defaultPorts(); err != nil {
			return err
		}
	}
	builder := e.builder(b, o.From)
	img, err := e.resolveImage(o.Image, f.Dir, builder)
	if err != nil {
		return err
	}
	// Asked before the build, which can take minutes.
	net := netpolicy.Choose(o.NetworkPolicy, o.NoNetworkPolicy, vm)
	// Every VM has the share, read-only: a tool added later is mounted
	// from it. Its agents' folders on the host are mounted read-only.
	spec := backend.VMSpec{
		Name: vm, Project: f.Dir, AgentDirs: AgentMounts(e.Home, img), Tools: e.Dirs.Tools(), ToolsAt: tools.ShareAt,
		AgentForwarding: o.SSHAgent, Size: size.Backend(), Ports: PortsBackend(ports),
	}
	agent := sshagent.Off
	if o.SSHAgent {
		agent = sshagent.On
	}
	e.useAgent(agent)
	mode := ""
	if o.Clone {
		spec.Project, mode = "", ModeClone
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if net {
		conf.Net = backend.NetProxy
	}
	need := append(spec.Features(), conf.Features()...)
	if o.Clone {
		// Its clone is copied in and fetched over ssh.
		need = append(need, backend.FeatureSSH)
	}
	if err := backend.Require(b, vm, need...); err != nil {
		return err
	}
	// A new VM gets the newest tools: check now when the daily check is
	// due.
	groups := image.ToolsOf(img)
	if len(groups) > 0 && e.Tools.CheckDue() {
		e.Tools.Check(ctx)
	}
	if err := builder.Build(ctx, img); err != nil {
		return err
	}
	chain, err := builder.Chain(image.WithoutTools(img))
	if err != nil {
		return err
	}
	tg, err := builder.ToolTarget()
	if err != nil {
		return err
	}
	libc := tg.Libc
	files, err := e.Tools.Files(ctx, groups, tg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.Dirs.Tools(), 0o700); err != nil {
		return err
	}
	id, err := platform.FolderID(f.Dir)
	if err != nil {
		return err
	}

	ui.Info(msg.CreatingFrom(img, vm))
	ui.LogNote()
	// Until its first shell nothing of anyone's is in it: an interrupt
	// deletes it and its state, and takes off .git's locks, or removes
	// the repo's remote for its clone.
	var locking atomic.Bool
	done := ui.AddCleanup(func(ctx context.Context) {
		if !ui.Interrupted() {
			return
		}
		ui.Info(msg.InterruptedVM(vm))
		if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			ui.Warn(msg.DeleteFailed(vm))
		}
		if locking.Load() {
			e.Protector(f.Dir).Unlock(ctx)
		}
		if st, err := state.ReadVM(e.Dirs, vm); err == nil && isClone(st) {
			removeRemote(ctx, f.Dir, vm, st.Get(KeyCloneRemote))
		}
		_ = e.sshConfig().ForgetHostKeys(vm)
		_ = e.Proxy.Forget(vm)
		_ = os.Remove(e.Dirs.VMState(vm))
	})
	// The state first, with the network choice: if a later step fails,
	// the VM never starts without it.
	st, err := state.CreateVM(e.Dirs, vm, state.NewVM{
		Image: img, Dir: f.Dir, Folder: id, Chain: chain.String(), From: o.From, Backend: b.Name(),
	})
	if err != nil {
		return err
	}
	// What a VM by this name deleted some other way left: its proxy, its
	// own rules, its run files.
	if err := e.Proxy.Forget(vm); err != nil {
		return err
	}
	netState := "off"
	if net {
		netState = "on"
	}
	for _, kv := range [][2]string{
		{KeyNet, netState},
		{KeyMode, mode},
		{sshagent.Key, agent},
		{KeyLibc, libc},
		{KeyTools, strings.Join(files, " ")},
		{KeyCPUs, size.CPUs},
		{KeyMemory, size.Memory},
		{KeyDisk, size.Disk},
	} {
		if kv[1] == "" {
			continue
		}
		if err := st.Set(kv[0], kv[1]); err != nil {
			return err
		}
	}
	if err := b.CreateVM(ctx, chain.Key(), spec); err != nil {
		return err
	}
	if ports != "" && ports != PortsAll {
		for _, k := range []string{KeyPorts, KeyPortsSet} {
			if err := st.Set(k, ports); err != nil {
				return err
			}
		}
	}
	// A clone has no .git of the host's to protect, and no origin.
	if !o.Clone {
		locking.Store(!o.NoGitProtect)
		if err := e.Protector(f.Dir).NewVM(ctx, st, o.NoGitProtect); err != nil {
			return err
		}
		if !o.SSHAgent && project.SSHRemote(ctx, f.Dir) {
			ui.Warn(msg.SSHRemoteHint(vm))
		}
		// Files an earlier VM of this folder made are named too.
		e.checkSetID(f.Dir)
	}
	if spec.AgentForwarding {
		e.checkAgent(ctx)
	}
	ui.Info(msg.Starting(vm))
	// The confinement the VM last started with, its proxy's address in it.
	var started backend.Confinement
	start := func(ctx context.Context) error {
		var err error
		started, err = e.start(ctx, b, st, conf)
		return err
	}
	if err := start(ctx); err != nil {
		return err
	}
	// Before the setup and the shell.
	if err := netpolicy.SelfCheck(ctx, b, st, start); err != nil {
		return err
	}
	cmd, err := e.setUp(ctx, b, st, f, started)
	if err != nil {
		return err
	}
	done()
	return e.Exec(cmd)
}

// setUp sets up a running VM, started with conf, for a shell in the
// folder (setUpVM), and returns the shell's command.
func (e *Env) setUp(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) (backend.Command, error) {
	if err := e.setUpVM(ctx, b, st, f, conf); err != nil {
		return backend.Command{}, err
	}
	return b.Shell(ctx, st.Name(), f.Dir)
}

// setUpVM sets up a running VM, started with conf: its setup sent, ssh
// and editors reaching it by name, and a --clone VM's clone made if it
// isn't yet.
func (e *Env) setUpVM(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) error {
	if err := e.sync(ctx, b, st, f.Slug(), strings.Fields(st.Get(KeyTools)), conf); err != nil {
		return err
	}
	if err := e.sshConfig().Write(ctx); err != nil {
		return err
	}
	if isClone(st) && st.Get(KeySeeded) == "" {
		return e.seedClone(ctx, b, st, f)
	}
	return nil
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
	if err := e.sshConfig().ForgetHostKeys(vm); err != nil {
		return conf, err
	}
	// The VM's backend forwards the agent it gets when it starts: devvm's
	// own, with the keys it lacks loaded, for a VM that uses it.
	if sshagent.Mode(st) == sshagent.Devvm {
		if err := e.Agent.Ensure(ctx); err != nil {
			return conf, err
		}
		conf.AgentSocket = e.Agent.Socket()
	}
	if conf.Net != backend.NetProxy {
		e.Proxy.Stop(vm)
	} else {
		p, err := e.Proxy.Prepare(ctx, b, st, false)
		if err != nil {
			return conf, err
		}
		// The proxy's confinement, with the agent chosen above.
		p.AgentSocket = conf.AgentSocket
		conf = p
	}
	if err := b.Start(ctx, vm, conf); err != nil {
		if conf.Net == backend.NetProxy {
			e.Proxy.Stop(vm)
		}
		return conf, err
	}
	return conf, nil
}

// useAgent makes the SSH agent devvm's own connections to the folder's VM
// carry the one its mode says: devvm's, the user's, or none. Its backend's
// connections to it (Lima's share one) and the shell's are among them.
func (e *Env) useAgent(mode string) {
	switch mode {
	case sshagent.Devvm:
		_ = os.Setenv("SSH_AUTH_SOCK", e.Agent.Socket())
	case sshagent.On:
	default:
		_ = os.Unsetenv("SSH_AUTH_SOCK")
	}
}

// mounts returns the host folders project VMs mount, as their state and
// backends have them: the project folder (not a --clone VM's), agent
// folders, the tools share. A backend that can't list runs no VMs.
func (e *Env) mounts(ctx context.Context) ([]sshagent.Mount, error) {
	var ms []sshagent.Mount
	for _, b := range e.Registry.All() {
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
func (e *Env) sshConfig() *sshconfig.Config { return sshconfig.New(e.Dirs, e.Registry) }

// checkUnsafe refuses a folder that is or holds the home folder or
// devvm's state, or is inside devvm's own folders, unless the user means
// it (unsafe).
func (e *Env) checkUnsafe(dir string, unsafe bool) error {
	u := project.Unsafe{Home: e.Home, Config: e.Dirs.Config, Data: e.Dirs.Data, Lima: e.Dirs.Lima}
	if holds := u.Holds(dir); holds != "" {
		if !unsafe {
			return msg.UnsafeFolder(dir, holds)
		}
		ui.Warn(msg.UnsafeFolderUsed(dir, holds))
	}
	if inside := u.Inside(dir); inside != "" {
		if !unsafe {
			return msg.UnsafeFolderInside(dir, inside)
		}
		ui.Warn(msg.UnsafeFolderUsed(dir, inside))
	}
	return nil
}

// resolveImage returns the image a new VM gets, in canonical order: the
// one named, or with nothing but agents named, the folder's toolchains
// and the agents on top (devvm claude in a node project is node+claude).
// base is a choice of its own: nothing is detected over it.
func (e *Env) resolveImage(want, dir string, b *build.Builder) (string, error) {
	if needsDetection(want) {
		found := project.Detect(dir)
		switch {
		case len(found) > 0:
			ui.Info(msg.Detected(strings.Join(found, "+")))
			if want != "" {
				found = append(found, want)
			}
			want = strings.Join(found, "+")
		case want == "":
			return "", msg.NoImageFor(strings.Join(image.Agents, " "), dir, strings.Join(b.Environments(), " "))
		}
	}
	img := image.Normalize(want)
	return img, b.CheckLayers(img)
}

// needsDetection reports whether an image name names only agents, or
// nothing: the folder's toolchains then go under them.
func needsDetection(want string) bool {
	if want == "" {
		return true
	}
	for p := range strings.SplitSeq(want, "+") {
		if !image.IsAgent(p) {
			return false
		}
	}
	return true
}
