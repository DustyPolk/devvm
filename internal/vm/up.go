// Package vm runs project VMs: up, which makes a folder's VM from its image
// and opens a shell in it, and the sizes and ports VMs get.
package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/secrets"
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
	// What a VM's setup sends of the host's settings: the environment
	// (TZ, LANG, LC_ALL), the timezone, and git's settings in a folder.
	Getenv    func(string) string
	HostTZ    func() string
	GitConfig func(ctx context.Context, dir, key string) string
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
	return &Env{
		Dirs: d, Root: root, Registry: reg, Tools: tools.New(d, root),
		Proxy: netpolicy.NewProxy(d, root, self, secrets.NewStore(d, home)),
		Dir:   wd, Home: home, OS: runtime.GOOS,
		Host: ThisHost, Protector: gitprotect.New, Exec: execShell,
		Getenv: os.Getenv, HostTZ: hostTZ, GitConfig: gitConfig,
	}, nil
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
// protected, started. On success it doesn't return: the shell takes
// devvm's place.
func (e *Env) Up(ctx context.Context, o Options) error {
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
	// The VM's name fits a new VM's backend; a VM that exists keeps the
	// backend its state records.
	first, err := e.Registry.For(e.Dirs, new(state.VM), o.Backend)
	if err != nil {
		return err
	}
	f, err := project.New(e.Dir, first.Backend.NameMax())
	if bad, ok := errors.AsType[*project.BadPathError](err); ok {
		return msg.BadPath(msg.Shown(bad.Dir))
	} else if errors.Is(err, project.ErrNoRoom) {
		return msg.VMNameNoRoom(first.Backend.Name(), strconv.Itoa(first.Backend.NameMax()))
	} else if err != nil {
		return err
	}
	st, err := state.ReadVM(e.Dirs, f.Name)
	if err != nil {
		return err
	}
	choice, err := e.Registry.For(e.Dirs, st, o.Backend)
	if err != nil {
		return err
	}
	b := choice.Backend
	if err := b.Check(ctx); err != nil {
		return err
	}
	switch _, err := b.Inspect(ctx, f.Name); {
	case err == nil:
		if err := e.checkFolder(f, st); err != nil {
			return err
		}
		return msg.VMExists(f.Name)
	case !errors.Is(err, backend.ErrNotFound):
		return err
	}
	return e.create(ctx, o, f, b, size, ports)
}

// checkFolder refuses a VM made for another folder at the same path: one
// deleted and made again (another repo cloned there) never quietly gets
// the old one's disk.
func (e *Env) checkFolder(f project.Folder, st *state.VM) error {
	if !st.Exists() {
		return nil
	}
	id, err := platform.FolderID(f.Dir)
	if err != nil {
		return err
	}
	if st.Get(state.KeyDir) != f.Dir || st.Get(state.KeyFolder) != id {
		return msg.VMOtherFolder(f.Name, st.Get(state.KeyDir))
	}
	return nil
}

// create makes the folder's VM and opens a shell in it.
func (e *Env) create(ctx context.Context, o Options, f project.Folder, b backend.Backend, size Size, ports string) error {
	vm := f.Name
	if err := e.checkUnsafe(f.Dir, o.UnsafeFolder); err != nil {
		return err
	}
	// Defaults are checked again: the files may be edited by hand. All
	// before the build, which can take minutes.
	def, err := state.ReadSize(e.Dirs)
	if err != nil {
		return err
	}
	if size, err = size.Or(Size{CPUs: def.CPUs, Memory: def.Memory, Disk: def.Disk}).Parse(); err != nil {
		return err
	}
	host, err := e.Host()
	if err != nil {
		return err
	}
	if err := size.Check(host, BaseDisk(e.Root), vm); err != nil {
		return err
	}
	if ports == "" {
		def, err := state.ReadPorts(e.Dirs)
		if err != nil {
			return err
		}
		if def != "" {
			if ports, err = ParsePorts(def); err != nil {
				return err
			}
		}
	}
	builder := &build.Builder{Dirs: e.Dirs, Root: e.Root, Backend: b, Tools: e.Tools}
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
		Size: size.Backend(), Ports: PortsBackend(ports),
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if net {
		conf.Net = backend.NetProxy
	}
	if err := backend.Require(b, vm, append(spec.Features(), conf.Features()...)...); err != nil {
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
	chain, err := builder.Inputs().Chain(image.WithoutTools(img))
	if err != nil {
		return err
	}
	libc, err := builder.BaseLibc()
	if err != nil {
		return err
	}
	files, err := e.Tools.Files(ctx, groups, libc)
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
	// deletes it and its state, and takes off .git's locks.
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
		_ = e.Proxy.Forget(vm)
		_ = os.Remove(e.Dirs.VMState(vm))
	})
	// The state first, with the network choice: if a later step fails,
	// the VM never starts without it.
	st, err := state.CreateVM(e.Dirs, vm, state.NewVM{
		Image: img, Dir: f.Dir, Folder: id, Chain: chain.String(), Backend: b.Name(),
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
	locking.Store(!o.NoGitProtect)
	if err := e.Protector(f.Dir).NewVM(ctx, st, o.NoGitProtect); err != nil {
		return err
	}
	if project.SSHRemote(ctx, f.Dir) {
		ui.Warn(msg.SSHRemoteHint(vm))
	}
	// On Linux hosts set-id bits reach the host (macOS's virtiofs drops
	// them): files an earlier VM of this folder made are named too.
	if e.OS == "linux" {
		if files := project.SetIDFiles(f.Dir); len(files) > 0 {
			var shown []string
			for _, p := range files {
				shown = append(shown, msg.Shown(p))
			}
			ui.Warn(msg.SetidFiles(strings.Join(shown, " ")))
		}
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
	if err := e.sync(ctx, b, st, f.Slug(), files, started); err != nil {
		return err
	}
	cmd, err := b.Shell(ctx, vm, f.Dir)
	if err != nil {
		return err
	}
	done()
	return e.Exec(cmd)
}

// start starts a stopped VM confined as conf says, and returns the
// confinement it got. With network policy its proxy starts first, and the
// backend gets the proxy's confinement; a start that fails stops the proxy
// again.
func (e *Env) start(ctx context.Context, b backend.Backend, st *state.VM, conf backend.Confinement) (backend.Confinement, error) {
	vm := st.Name()
	if conf.Net == backend.NetProxy {
		var err error
		if conf, err = e.Proxy.Prepare(ctx, st, b.Lifeline(vm), false); err != nil {
			return conf, err
		}
	}
	if err := b.Start(ctx, vm, conf); err != nil {
		if conf.Net == backend.NetProxy {
			e.Proxy.Stop(vm)
		}
		return conf, err
	}
	return conf, nil
}

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
