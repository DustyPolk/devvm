package vm

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
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

// Launch opens a shell, ssh or an editor in the folder's VM: a new VM
// made first (here), one that exists launched as it is, with what's
// changed since (existing.go).
type Launch struct{ *Env }

// Up opens a shell in the folder's VM, making it first: checked, from
// its image (built if it isn't), with only the folder mounted, .git
// protected, started; with --clone, nothing mounted but its own clone of
// the repo (seedClone). A VM that exists is opened as it is, with what's
// changed since checked (open). On success it doesn't return: the shell
// takes devvm's place.
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

// create makes the folder's VM and opens a shell in it.
func (e Launch) create(ctx context.Context, o Options, f project.Folder, b backend.Backend, size Size, ports string) error {
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
	net := e.Proxy.Choose(o.NetworkPolicy, o.NoNetworkPolicy, vm)
	// Every VM has the share, read-only: a tool added later is mounted
	// from it. Its agents' folders on the host are mounted read-only.
	spec := backend.VMSpec{
		Name: vm, Project: f.Dir, AgentDirs: e.AgentMounts(img), Tools: e.Dirs.Tools(), ToolsAt: tools.ShareAt,
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
	if err := e.Registry.Require(b, vm, need...); err != nil {
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

	e.Info(msg.CreatingFrom(img, vm))
	e.LogNote()
	// Until its first shell nothing of anyone's is in it: an interrupt
	// deletes it and its state, and takes off .git's locks, or removes
	// the repo's remote for its clone.
	var locking atomic.Bool
	done := e.AddCleanup(func(ctx context.Context) {
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
			clones(e).removeRemote(ctx, f.Dir, vm, st.Get(KeyCloneRemote))
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
			e.Warn(msg.SSHRemoteHint(vm))
		}
		// Files an earlier VM of this folder made are named too.
		e.checkSetID(f.Dir)
	}
	if spec.AgentForwarding {
		e.checkAgent(ctx)
	}
	e.Info(msg.Starting(vm))
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
	if err := e.Proxy.SelfCheck(ctx, b, st, start); err != nil {
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
func (e Launch) setUp(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) (backend.Command, error) {
	if err := e.setUpVM(ctx, b, st, f, conf); err != nil {
		return backend.Command{}, err
	}
	return b.Shell(ctx, st.Name(), f.Dir)
}

// setUpVM sets up a running VM, started with conf: its setup sent, ssh
// and editors reaching it by name, and a --clone VM's clone made if it
// isn't yet.
func (e Launch) setUpVM(ctx context.Context, b backend.Backend, st *state.VM, f project.Folder, conf backend.Confinement) error {
	if err := syncer(e).sync(ctx, b, st, f.Slug(), strings.Fields(st.Get(KeyTools)), conf); err != nil {
		return err
	}
	if err := e.sshConfig().Write(ctx); err != nil {
		return err
	}
	if isClone(st) && st.Get(KeySeeded) == "" {
		return clones(e).seedClone(ctx, b, st, f)
	}
	return nil
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
// base is a choice of its own: nothing is detected over it.
func (e Launch) resolveImage(want, dir string, b *build.Builder) (string, error) {
	if needsDetection(want) {
		found := project.Detect(dir)
		switch {
		case len(found) > 0:
			e.Info(msg.Detected(strings.Join(found, "+")))
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
