package vm

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
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
	FromSet         bool   // --from was given, not its default
	NoDevcontainer  bool   // --no-devcontainer: a new VM ignores the folder's devcontainer.json
	Here            bool   // --here: a new VM for this folder, without a parent folder's VM asked about
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
// changed since checked (open). When the folder has none but a parent
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
		if ports, err = ParsePorts(o.Ports); err != nil {
			return err
		}
	}
	// --clone asks for a new VM of the folder's own: as --here.
	t, parent, err := e.lookup(ctx, o.Backend, !o.Here && !o.Clone)
	if err != nil {
		return err
	}
	if parent != nil && useParent(e.Env, parent) {
		t = parent
	}
	if !t.exists {
		return creation{e}.create(ctx, o, t.f, t.b, size, ports)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return err
	}
	return e.open(ctx, o, t)
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
	img   string
	size  Size
	ports string
	net   bool
	agent string // sshagent.On or Off
	mode  string // ModeClone, or ""
	// dockerLayer is how it runs the docker layer (KeyDockerLayer).
	dockerLayer string
	spec        backend.VMSpec
	conf        backend.Confinement
	builder     *build.Env
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
	// devvm gc --tools nor retention deletes any of them meanwhile.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	defer unmaking()
	// A new VM gets the newest tools: check now when the daily check is
	// due.
	groups := image.ToolsOf(n.img)
	versions := tools.Versions{Env: e.Tools}
	if len(groups) > 0 && versions.CheckDue() {
		versions.Check(ctx)
	}
	if err := (build.Builds{Env: n.builder}).Build(ctx, n.img); err != nil {
		return err
	}
	chain, err := n.builder.Chain(image.WithoutTools(n.img))
	if err != nil {
		return err
	}
	tg, err := n.builder.ToolTarget()
	if err != nil {
		return err
	}
	files, err := versions.Files(ctx, groups, tg)
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

	e.Info(msg.CreatingFrom(n.img, vm))
	e.LogNote()
	var locking atomic.Bool
	done := e.AddCleanup(e.undoCreate(b, f, &locking))
	// The state first, with the network choice: if a later step fails,
	// the VM never starts without it.
	st, err := state.CreateVM(e.Dirs, vm, state.NewVM{
		Image: n.img, Dir: f.Dir, Folder: id, Chain: chain.String(), From: o.From, Backend: b.Name(),
	})
	if err != nil {
		return err
	}
	if at, _ := backend.LocationOf(b); at != "" {
		if err := st.Set(KeyBackendAt, at); err != nil {
			return err
		}
	}
	// What a VM by this name deleted some other way left: its proxy, its
	// own rules, its run files.
	if err := e.Proxy.Forget(vm); err != nil {
		return err
	}
	if err := n.record(st, tg.Libc, files); err != nil {
		return err
	}
	if err := (devcontainers{e.Env}).record(st, o); err != nil {
		return err
	}
	if n.ports != "" && n.ports != PortsAll {
		if err := st.Set(KeyPorts, n.ports); err != nil {
			return err
		}
	}
	// Its ssh port from the start, as its starts have it: none of its
	// listed ports, recorded above.
	if n.spec.SSHPort, err = e.Proxy.SSHPort(ctx, st, false); err != nil {
		return err
	}
	if err := b.CreateVM(ctx, chain.Key(), n.spec); err != nil {
		return err
	}
	if n.spec.AgentToggle && b.Caps()[backend.FeatureAgentToggle].IsEmulated() {
		if err := st.Set(KeyAgentToggle, "on"); err != nil {
			return err
		}
	}
	// A clone has no .git of the host's to protect, and no origin.
	if !o.Clone {
		locking.Store(!o.NoGitProtect)
		if err := e.Protector(f.Dir).NewVM(ctx, st, o.NoGitProtect); err != nil {
			return err
		}
		if !o.SSHAgent && project.SSHRemote(ctx, f.Dir) {
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

// plan checks and chooses what a new VM gets, all before the build,
// which can take minutes: its image, size, ports, network policy (asked
// for), SSH agent and the backend features it needs, with what the
// folder's devcontainer.json says (which changes o).
func (e creation) plan(ctx context.Context, o *Options, f project.Folder, b backend.Backend, size Size, ports string) (newVM, error) {
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
	if err := e.firstRunSize(size); err != nil {
		return newVM{}, err
	}
	// Defaults are checked again: the files may be edited by hand.
	def, err := e.defaultSize()
	if err != nil {
		return newVM{}, err
	}
	n := newVM{size: size.Or(def), ports: ports, agent: sshagent.Off, conf: backend.Confinement{Net: backend.NetDirect}}
	// A default disk size on a backend that sizes no disks (Docker's) is
	// left out, said; --disk is refused below.
	if size.Disk == "" && n.size.Disk != "" && !e.Registry.Allowed(b, backend.FeatureDisk) {
		e.Warn(msg.SizeDiskDefaultUnused(b.Name(), vm))
		n.size.Disk = ""
	}
	host, err := e.Host()
	if err != nil {
		return newVM{}, err
	}
	if err := n.size.Check(host, BaseDisk(e.Assets), vm); err != nil {
		return newVM{}, err
	}
	if n.ports == "" {
		if n.ports, err = e.defaultPorts(); err != nil {
			return newVM{}, err
		}
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
	n.builder = e.builder(b, o.From)
	if n.img, err = e.resolveImage(o.Image, f.Dir, o.dc, n.builder); err != nil {
		return newVM{}, err
	}
	// Every VM has the share, read-only: a tool added later is mounted
	// from it. Its agents' folders on the host are mounted read-only.
	n.spec = backend.VMSpec{
		Name: vm, Project: f.Dir, AgentDirs: e.AgentMounts(n.img), Tools: e.Dirs.Tools(), ToolsAt: tools.ShareAt,
		AgentForwarding: o.SSHAgent, Size: n.size.Backend(), Ports: PortsBackend(n.ports),
		Env: trust.GuestEnv(nil),
	}
	layerNeeds, err := e.dockerLayer(b, &n)
	if err != nil {
		return newVM{}, err
	}
	if n.net, err = e.chooseNet(o, n.dockerLayer != "", b, vm); err != nil {
		return newVM{}, err
	}
	if o.SSHAgent {
		n.agent = sshagent.On
	}
	e.useAgent(n.agent)
	if o.Clone {
		n.spec.Project, n.spec.CloneAt, n.mode = "", f.Dir, ModeClone
	}
	if n.net {
		n.conf.Net = backend.NetProxy
	}
	need := slices.Concat(n.spec.Features(), n.conf.Features(), layerNeeds)
	if o.Clone {
		// Its clone is copied in and fetched over ssh.
		need = append(need, backend.FeatureSSH)
	}
	if err := e.Registry.Require(b, vm, need...); err != nil {
		return newVM{}, err
	}
	dockerLayers{e.Env}.say(b, n.dockerLayer, vm)
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
// the saved choice's (devvm network -g, devvm setup), which isn't read
// when a flag is given, else asked for, on by default. A saved off makes
// the VM without it, unsaid. A VM that can't have it is made without it,
// unasked and said: root in a VM made for the docker layer (dockerVM),
// named or from a devcontainer.json's docker-in-docker, can change its
// own network, and b may not confine a VM to its proxy (egress-proxy).
// --network-policy on such a VM is refused, here or by Require, never
// dropped.
func (e creation) chooseNet(o *Options, dockerVM bool, b backend.Backend, vm string) (bool, error) {
	saved := state.NetworkAsk
	if !o.NetworkPolicy && !o.NoNetworkPolicy {
		var err error
		if saved, err = state.ReadNetwork(e.Dirs); err != nil {
			return false, err
		}
	}
	switch {
	case o.NoNetworkPolicy, saved == state.NetworkOff:
		return false, nil
	case dockerVM && o.NetworkPolicy:
		return false, msg.NetDockerLayer(b.Name(), vm)
	case dockerVM:
		e.Warn(msg.NetOffDockerLayer(b.Name(), vm))
		return false, nil
	case !o.NetworkPolicy && !e.Registry.Allowed(b, backend.FeatureEgressProxy):
		e.Warn(msg.NetOffBackend(b.Name(), vm))
		return false, nil
	case o.NetworkPolicy:
		return true, nil
	}
	return e.Proxy.Choose(saved == state.NetworkOn, false, vm), nil
}

// dockerLayer chooses the runtime a new VM runs the docker layer under,
// when its image has it, on a backend that makes VMs for it
// (dockerLayers.choice), and returns the features its layers need.
func (e creation) dockerLayer(b backend.Backend, n *newVM) ([]backend.Feature, error) {
	dl, made := b.(backend.DockerLayerVM)
	if !made || !image.HasLayer(n.img, image.DockerLayer) {
		return backend.ImageFeatures(n.img), nil
	}
	mode, need, err := dockerLayers{e.Env}.choice(dl, n.spec.Name, n.img)
	if err != nil {
		return nil, err
	}
	n.spec.DockerLayer, n.dockerLayer = true, mode
	return need, nil
}

// record writes a new VM's choices into its state, with the libc and
// tool files it was given.
func (n newVM) record(st *state.VM, libc string, files []string) error {
	netState := "off"
	if n.net {
		netState = "on"
	}
	for _, kv := range [][2]string{
		{KeyNet, netState},
		{KeyMode, n.mode},
		{KeyDockerLayer, n.dockerLayer},
		{sshagent.Key, n.agent},
		{KeyLibc, libc},
		{KeyTools, strings.Join(files, " ")},
		{KeyCPUs, n.size.CPUs},
		{KeyMemory, n.size.Memory},
		{KeyDisk, n.size.Disk},
	} {
		if kv[1] == "" {
			continue
		}
		if err := st.Set(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// undoCreate is create's cleanup. Until its first shell nothing of
// anyone's is in the VM: an interrupt deletes it and its state, and
// takes off .git's locks (once locking is set), or removes the repo's
// remote for its clone.
func (e creation) undoCreate(b backend.Backend, f project.Folder, locking *atomic.Bool) func(context.Context) {
	vm := f.Name
	return func(ctx context.Context) {
		if !e.Interrupted() {
			return
		}
		e.Info(msg.InterruptedVM(vm))
		if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			e.Warn(msg.DeleteFailed(vm))
		}
		if locking.Load() {
			e.Protector(f.Dir).Unlock(ctx)
		}
		if st, err := state.ReadVM(e.Dirs, vm); err == nil && isClone(st) {
			clones{e.Env}.removeRemote(ctx, f.Dir, vm, st.Get(KeyCloneRemote))
		}
		_ = e.sshConfig().ForgetHostKeys(vm)
		_ = e.Proxy.Forget(vm)
		_ = os.Remove(e.Dirs.VMState(vm))
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
	if err := syncer(e).sync(ctx, b, st, f.Slug(), strings.Fields(st.Get(KeyTools)), conf); err != nil {
		return err
	}
	if err := e.sshConfig().Write(ctx); err != nil {
		return err
	}
	// Its links, once its host key is pinned, which their session checks.
	if err := linkRuns(e).atLaunch(ctx, b, st); err != nil {
		return err
	}
	if isClone(st) && st.Get(KeySeeded) == "" {
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
	u := project.Unsafe{Home: e.Home, Config: e.Dirs.Config, Data: e.Dirs.Data, Lima: e.Dirs.Lima}
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

// resolveImage returns the image a new VM gets, in canonical order: the
// one named, or with nothing but agents named, the folder's toolchains
// and the agents on top (devvm claude in a node project is node+claude).
// base is a choice of its own: nothing is detected over it. With a
// devcontainer.json (dc), the layers its features map to go under it, and
// nothing named or detected is base.
func (e Launch) resolveImage(want, dir string, dc *devcontainer.Plan, b *build.Env) (string, error) {
	var layers []string
	if dc != nil {
		layers = dc.Layers
	}
	if needsDetection(want) {
		found := project.Detect(dir)
		switch {
		case len(found) > 0:
			e.Info(msg.Detected(strings.Join(found, "+")))
			if want != "" {
				found = append(found, want)
			}
			want = strings.Join(found, "+")
		case want == "" && dc == nil:
			return "", msg.NoImageFor(strings.Join(image.Agents, " "), dir, strings.Join(b.Environments(), " "))
		}
	}
	var parts []string
	for _, p := range slices.Concat(layers, strings.Split(want, "+")) {
		if p != "" && !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	img := image.Normalize(strings.Join(parts, "+"))
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
