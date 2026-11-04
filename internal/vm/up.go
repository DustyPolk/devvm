package vm

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/hostsafety"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/sshconfig"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
)

// Options is what up was asked for.
type Options struct {
	// Toolchains, Agents and Layers are --tools, --agents and --layers:
	// each the exact list of its part a new VM gets, empty for none; nil
	// for the part's default.
	Toolchains, Agents, Layers *[]string
	NetworkPolicy              bool   // --network-policy
	NoNetworkPolicy            bool   // --no-network-policy
	Size                       Size   // --cpus, --memory, --disk as given
	Ports                      string // --ports as given
	UnsafeFolder               bool   // --unsafe-folder
	NoGitProtect               bool   // --no-git-protect
	Clone                      bool   // --clone
	SSHAgent                   bool   // --ssh-agent: a new VM forwards the user's SSH agent
	From                       string // --from, or its default: the container image a new VM's base starts from
	FromSet                    bool   // --from was given, not its default
	NoDevcontainer             bool   // --no-devcontainer: a new VM ignores the folder's devcontainer.json
	Here                       bool   // --here: a new VM for this folder, without a parent folder's VM asked about
	// dc is what the folder's devcontainer.json says a new VM gets
	// (devcontainers.prepare); nil without one.
	dc *devcontainer.Plan
}

// Launch opens a shell, ssh or an editor in the folder's VM: a new VM
// made first (here), one that exists launched as it is, with what's
// changed since (existing.go).
type Launch struct{ *Env }

// Up opens a shell in the folder's VM, making it first: checked, from
// its image (built if it isn't), with only the folder mounted, .git
// protected, started; with --clone, nothing mounted but its own clone of
// the repo (seedClone). A VM that exists is opened as it is, with what's
// changed since checked (open), and one whose remake didn't finish is
// finished first (remaking.finishPending). When the folder has none but a parent
// folder has one, that one is opened in this folder, after asking in a
// terminal (useParent), unless --here or --clone. On success it doesn't return: the
// shell takes devvm's place.
func (e Launch) Up(ctx context.Context, o Options) error {
	if err := from.Check(o.From); err != nil {
		return err
	}
	size, err := o.Size.Parse()
	if err != nil {
		return err
	}
	ports := ""
	if o.Ports != "" {
		if ports, err = settings.ParsePorts(o.Ports); err != nil {
			return err
		}
	}
	// --clone asks for a new VM of the folder's own: as --here.
	t, parent, err := e.lookup(ctx, !o.Here && !o.Clone)
	if err != nil {
		return err
	}
	if parent != nil && useParent(e.Env, parent) {
		t = parent
	}
	// A remake that didn't finish, whose machine may be gone: finished,
	// never made anew.
	if remakePending(t.st) {
		return e.finishRemake(ctx, o, t)
	}
	if !t.exists {
		// The company proxy its builds and its proxy go through, checked
		// before anything is built.
		if _, err := e.Proxy.Upstream(ctx); err != nil {
			return err
		}
		return creation{e}.create(ctx, o, t.f, t.b, size, ports)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return err
	}
	if remakeUnfinished(e.Env, t) {
		if err := Lifecycle(e).delete(ctx, t); err != nil {
			return err
		}
		return creation{e}.create(ctx, o, t.f, t.b, size, ports)
	}
	return e.open(ctx, o, t)
}

// finishRemake finishes t's VM's remake that didn't (remaking.finishPending),
// holding its remake lock, unless another devvm finished it meanwhile,
// then opens it.
func (e Launch) finishRemake(ctx context.Context, o Options, t *found) error {
	if err := e.checkFolder(t.f, t.st); err != nil {
		return err
	}
	release, err := remaking(e).hold(ctx, t)
	if err != nil {
		return err
	}
	defer release()
	if remakePending(t.st) {
		if err := remaking(e).finishPending(ctx, t); err != nil {
			return err
		}
	}
	return e.open(ctx, o, t)
}

// remakeUnfinished reports whether up deletes t, a VM whose creation
// didn't finish, and makes it again: asked in a terminal, without
// --no-input ([y/N]). It never ran a shell, so nothing of the user's is in
// it. A finished VM is opened; an unfinished one not asked about (a parent
// folder's, or no question) or kept is refused (vm_unfinished).
func remakeUnfinished(e *Env, t *found) bool {
	if t.st.Finished() || t.sub != "" || e.CanAsk == nil || !e.CanAsk() {
		return false
	}
	return e.Ask(msg.RemakeUnfinished(t.f.Name))
}

// useParent reports whether up opens the parent folder's VM p in this
// folder: asked in a terminal ([Y/n]; no makes a new VM for this folder),
// else yes, said in an info line. Below a repo of its own (p.repo), the
// default is the new VM: [y/N] in a terminal, else no.
func useParent(e *Env, p *found) bool {
	// A repo of its own below the parent's: a new VM unless asked for.
	if p.repo != "" {
		return e.Terminal() && e.Ask(msg.UseParentVMRepo(p.sub, p.f.Dir, p.repo, p.f.Name))
	}
	if !e.Terminal() {
		e.Info(msg.UsingParentVM(p.f.Dir, p.f.Name))
		return true
	}
	return e.Confirm(msg.UseParentVM(p.sub, p.f.Dir, p.f.Name))
}

// newVM is what create checked and chose for a new VM before building
// anything.
type newVM struct {
	img     string   // its image: base and its layers
	tools   []string // its shared tools
	size    Size
	ports   string
	net     bool
	agent   string // sshagent.On or Off
	clone   bool   // --clone
	git     string // its .git protection (gitprotect.Protector.NewMode), or gitprotect.Clone
	spec    backend.VMSpec
	conf    backend.Confinement
	builder *build.Env
}

// creation makes a new VM: its plan, its build and its cleanup.
type creation struct{ Launch }

// create makes the folder's VM and opens a shell in it.
func (e creation) create(ctx context.Context, o Options, f project.Folder, b backend.Backend, size Size, ports string) error {
	vm := f.Name
	n, err := e.plan(ctx, &o, f, b, size, ports)
	if err != nil {
		return err
	}
	// Held from before its tools are picked until it's made, so neither
	// devvm gc --tool-images nor retention deletes any of them meanwhile.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	defer unmaking()
	// A new VM gets the newest tools: check now when the daily check is
	// due.
	versions := tools.Versions{Env: e.Tools}
	if len(n.tools) > 0 && versions.CheckDue() {
		versions.Check(ctx)
	}
	if err := (build.Builds{Env: n.builder}).Build(ctx, n.img); err != nil {
		return err
	}
	chain, err := n.builder.Chain(n.img)
	if err != nil {
		return err
	}
	tg, err := n.builder.ToolTarget()
	if err != nil {
		return err
	}
	files, err := versions.Files(ctx, n.tools, tg)
	if err != nil {
		return err
	}
	if err := e.buildTools(ctx, n.builder, tg, files); err != nil {
		return err
	}
	if err := os.MkdirAll(e.Dirs.Tools(), 0o700); err != nil {
		return err
	}
	id, err := platform.FolderID(f.Dir)
	if err != nil {
		return err
	}

	e.Info(msg.CreatingFrom(n.img, vm))
	e.LogNote()
	// What a VM made for this folder before, deleted some other way, left
	// in its folder: the new one's state replaces it.
	if err := (vmFolders{e.Env}).clear(ctx, vm); err != nil {
		return err
	}
	var locking atomic.Bool
	done := e.AddCleanup(e.undoCreate(b, f, &locking))
	// Its whole state first, written once, with all it was asked for: if
	// a later step fails, it never starts, as its creation didn't finish
	// (Created).
	st, err := state.New(e.Dirs, vm)
	if err != nil {
		return err
	}
	st.Image, st.Dir, st.Folder, st.Chain, st.From, st.Backend = n.img, f.Dir, id, chain.String(), o.From, b.Name()
	st.MadeBy = e.Version
	st.BackendAt, _ = backend.LocationOf(b)
	if err := n.record(st, tg.Libc, files); err != nil {
		return err
	}
	if err := (devcontainers{e.Env}).record(st, o); err != nil {
		return err
	}
	// Its alias, chosen now for good, as its name was, under the claims
	// lock: two VMs made at once never take one.
	err = st.Create(ctx, func(st *state.VM) error {
		var err error
		st.Alias, err = sshconfig.ChooseAlias(e.Dirs, vm)
		return err
	})
	if err != nil {
		return err
	}
	// Its ssh port from the start, as its starts have it: none of its
	// listed ports, recorded above.
	if n.spec.SSHPort, err = e.Proxy.Ports().SSH(ctx, st, false); err != nil {
		return err
	}
	if err := b.CreateVM(ctx, chain.Key(), n.spec); err != nil {
		return err
	}
	if err := e.created(ctx, b, st, n.spec.AgentToggle); err != nil {
		return err
	}
	// A clone has no .git of the host's to protect, and no origin.
	if !o.Clone {
		locking.Store(!o.NoGitProtect)
		if err := e.Protector(f.Dir).NewVM(ctx, st); err != nil {
			return err
		}
		if !o.SSHAgent && hostsafety.SSHRemote(ctx, f.Dir) {
			e.Warn(msg.SSHRemoteHint(vm))
		}
		// Files an earlier VM of this folder made are named too.
		e.checkSetID(f.Dir)
	}
	if n.spec.AgentForwarding {
		e.checkAgent(ctx)
	}
	e.Info(msg.Starting(vm))
	// The confinement the VM last started with, its proxy's address in it.
	var started backend.Confinement
	start := func(ctx context.Context) error {
		var err error
		started, err = e.start(ctx, b, st, n.conf)
		return err
	}
	if err := start(ctx); err != nil {
		return err
	}
	// Before the setup and the shell.
	if err := e.Proxy.SelfCheck(ctx, b, st, start); err != nil {
		return err
	}
	cmd, err := e.setUp(ctx, b, st, f, started)
	if err != nil {
		return err
	}
	done()
	unmaking()
	return e.Exec(cmd)
}

// created records that the backend made the VM, with toggle when it was
// made ready for devvm ssh-agent to change its forwarding and the backend
// emulates that, which its starts rely on: from now on it starts.
func (e creation) created(ctx context.Context, b backend.Backend, st *state.VM, toggle bool) error {
	toggle = toggle && b.Caps()[backend.FeatureAgentToggle].IsEmulated()
	return st.Update(ctx, func(st *state.VM) error {
		st.Created = time.Now().UTC()
		if !toggle {
			return nil
		}
		a, err := sshagent.Of(st)
		if err != nil {
			return err
		}
		a.Made.Toggle = true
		return sshagent.Set(st, a)
	})
}

// firstRun asks devvm's first run's questions, new VMs' size and agents
// (firstRunSize, firstRunAgents), then returns the defaults in effect, a
// snapshot the rest of the plan reads: the files may be edited by hand
// while it runs.
func (e creation) firstRun(o Options, size Size) (settings.Defaults, error) {
	def, err := settings.Read(e.Dirs)
	if err != nil {
		return def, err
	}
	if err := e.firstRunSize(size, def); err != nil {
		return def, err
	}
	if err := e.firstRunAgents(o, def); err != nil {
		return def, err
	}
	return settings.Read(e.Dirs)
}

// plan checks and chooses what a new VM gets, all before the build,
// which can take minutes: its image, size, ports, network policy (asked
// for), SSH agent and the backend features it needs, with what the
// folder's devcontainer.json says (which changes o).
func (e creation) plan(ctx context.Context, o *Options, f project.Folder, b backend.Backend, size Size, list string) (newVM, error) {
	vm := f.Name
	if o.Clone {
		if err := checkCloneRepo(ctx, f.Dir); err != nil {
			return newVM{}, err
		}
	} else if err := e.checkUnsafe(f.Dir, o.UnsafeFolder); err != nil {
		return newVM{}, err
	}
	if err := (devcontainers{e.Env}).prepare(ctx, o, vm, f.Dir); err != nil {
		return newVM{}, err
	}
	def, err := e.firstRun(*o, size)
	if err != nil {
		return newVM{}, err
	}
	n := newVM{size: size.Or(defaultSize(def)), ports: list, agent: sshagent.Off, conf: backend.Confinement{Net: backend.NetDirect}}
	host, err := e.Host()
	if err != nil {
		return newVM{}, err
	}
	if err := n.size.Check(host, BaseDisk(e.Assets), vm); err != nil {
		return newVM{}, err
	}
	if n.ports == "" {
		n.ports = def.Ports.Value
	}
	// The user's ports stay a limit the file can't widen: its ports
	// beyond them are said, not added.
	if o.dc != nil && o.dc.Ports != "" {
		wider, err := AddPorts(n.ports, o.dc.Ports)
		if err != nil {
			return newVM{}, err
		}
		if wider != n.ports {
			e.Warn(msg.DevcontainerPortsNotForwarded(o.dc.File, o.dc.Ports))
		}
	}
	forward, err := ports.Forwarding(n.ports)
	if err != nil {
		return newVM{}, err
	}
	n.builder = e.builder(b, o.From)
	parts, err := e.chooseParts(*o, vm, f.Dir, o.dc, n.builder, def)
	if err != nil {
		return newVM{}, err
	}
	n.img, n.tools = parts.image(), parts.tools()
	// Every VM has the share, read-only: a tool added later is mounted
	// from it. Its agents' folders on the host are mounted read-only.
	n.spec = backend.VMSpec{
		Name: vm, Project: f.Dir, AgentDirs: e.AgentMounts(n.tools), Tools: e.Dirs.Tools(), ToolsAt: tools.ShareAt,
		AgentForwarding: o.SSHAgent, Size: n.size.Backend(),
		Env: trust.GuestEnv(nil),
	}
	n.net = e.chooseNet(o, b, vm, def.Network.Value)
	if o.SSHAgent {
		n.agent = sshagent.On
	}
	e.useAgent(n.agent)
	if o.Clone {
		n.spec.Project, n.spec.CloneAt, n.clone, n.git = "", f.Dir, true, gitprotect.Clone
	} else {
		n.git = e.Protector(f.Dir).NewMode(o.NoGitProtect)
	}
	if n.net {
		n.conf.Net = backend.NetProxy
	}
	// Its ports, which devvm's forwarder forwards from the backend's ssh.
	need := slices.Concat(n.spec.Features(), forward.Features(), n.conf.Features(), backend.ImageFeatures(n.img))
	if o.Clone {
		// Its clone is copied in and fetched over ssh.
		need = append(need, backend.FeatureSSH)
	}
	if err := e.Registry.Require(b, vm, need...); err != nil {
		return newVM{}, err
	}
	// Ready for devvm ssh-agent to change later, where it may be changed
	// (an emulation once it's reviewed); otherwise fixed when it's made.
	n.spec.AgentToggle = e.Registry.Require(b, vm, backend.FeatureAgentToggle) == nil
	// Room for the VM, before anything is built or made; each build checks
	// its own.
	if err := (backend.Space{Free: e.FreeSpace}).Check(b, vm, func(f backend.Floors) uint64 { return f.VM }); err != nil {
		return newVM{}, err
	}
	return n, nil
}

// chooseNet returns whether a new VM gets network policy: the flags', else
// the default's (config.toml's, else devvm network -g's or devvm setup's),
// else asked for, on by default. A default of off makes the VM without
// it, unsaid. A VM whose backend b may not confine it to its proxy
// (egress-proxy) is made without it, unasked and said; --network-policy
// on such a VM is refused by Require, never dropped.
func (e creation) chooseNet(o *Options, b backend.Backend, vm, saved string) bool {
	if o.NetworkPolicy || o.NoNetworkPolicy {
		saved = settings.NetworkAsk
	}
	switch {
	case o.NoNetworkPolicy, saved == settings.NetworkOff:
		return false
	case !o.NetworkPolicy && !e.Registry.Allowed(b, backend.FeatureEgressProxy):
		e.Warn(msg.NetOffBackend(b.Name(), vm))
		return false
	case o.NetworkPolicy:
		return true
	}
	return e.Proxy.Choose(saved == settings.NetworkOn, false, vm)
}

// record sets a new VM's choices in its state, before it's written, with
// the libc and tool files it was given: each package's section.
func (n newVM) record(st *state.VM, libc string, files []string) error {
	forward := n.ports
	if forward == settings.PortsAll {
		forward = ""
	}
	for _, err := range []error{
		netpolicy.Set(st, netpolicy.Net{On: n.net}),
		gitprotect.Set(st, gitprotect.InVM{Mode: n.git}),
		sshagent.Set(st, sshagent.InVM{Mode: n.agent}),
		tools.Set(st, tools.InVM{Names: n.tools, Made: tools.Made{Files: files, Libc: libc}}),
		ports.Set(st, ports.Ports{Forward: forward}),
		state.SetSection(st, sizeSection, n.size),
		state.SetSection(st, cloneSection, cloneInVM{On: n.clone}),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// undoCreate is create's cleanup. Until its first shell nothing of
// anyone's is in the VM: an interrupt deletes it, then what devvm keeps
// for it (teardown), and takes off .git's locks (once locking is set,
// even when its state can't be read), or removes the repo's remote for
// its clone.
func (e creation) undoCreate(b backend.Backend, f project.Folder, locking *atomic.Bool) func(context.Context) {
	vm := f.Name
	return func(ctx context.Context) {
		if !e.Interrupted() {
			return
		}
		e.Info(msg.InterruptedVM(vm))
		deleted := true
		if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			e.Warn(msg.DeleteFailed(vm))
			deleted = false
		}
		st, err := state.ReadVM(e.Dirs, vm)
		if err != nil {
			st = new(state.VM)
		}
		if m := gitprotect.Mode(st); locking.Load() && m != gitprotect.On && m != gitprotect.Strict {
			e.Protector(f.Dir).Unlock(ctx, st)
		}
		if deleted {
			_ = vmFolders{e.Env}.teardown(ctx, &found{f: f, st: st, b: b}, false)
		}
	}
}

// setUp sets up a running VM, started with conf, for a shell in the
// folder (setUpVM), and returns the shell's command.
func (e Launch) setUp(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) (backend.Command, error) {
	if err := e.setUpVM(ctx, b, st, f, conf); err != nil {
		return backend.Command{}, err
	}
	return b.Shell(ctx, st.Name(), f.Dir)
}

// setUpVM sets up a running VM, started with conf: its setup sent, ssh
// and editors reaching it by name, its links carried (linkRuns.atLaunch),
// a --clone VM's clone made if it isn't yet, then its devcontainer.json's
// commands run (devcontainers.run).
func (e Launch) setUpVM(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) error {
	if err := syncer(e).sync(ctx, b, st, f.Slug(), tools.Files(st), conf); err != nil {
		return err
	}
	if err := e.sshConfig().Write(ctx); err != nil {
		return err
	}
	// Its links, once its host key is pinned, which their session checks.
	if err := linkRuns(e).atLaunch(ctx, b, st); err != nil {
		return err
	}
	if c := sectionOf[cloneInVM](st, cloneSection); c.On && !c.Made.Seeded {
		if err := clones(e).seedClone(ctx, b, st, f); err != nil {
			return err
		}
	}
	return devcontainers(e).run(ctx, b, st, f.Dir)
}

// checkUnsafe refuses a folder that is or holds the home folder or
// devvm's state, or is inside devvm's own folders, unless the user means
// it (unsafe).
func (e Launch) checkUnsafe(dir string, unsafe bool) error {
	u := project.Unsafe{Home: e.Dirs.Home, Config: e.Dirs.Config, Data: e.Dirs.Data, Lima: e.Dirs.Lima}
	if holds := u.Holds(dir); holds != "" {
		if !unsafe {
			return msg.UnsafeFolder(dir, holds)
		}
		e.Warn(msg.UnsafeFolderUsed(dir, holds))
	}
	if inside := u.Inside(dir); inside != "" {
		if !unsafe {
			return msg.UnsafeFolderInside(dir, inside)
		}
		e.Warn(msg.UnsafeFolderUsed(dir, inside))
	}
	return nil
}
