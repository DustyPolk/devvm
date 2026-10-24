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
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ui"
)

// Keys of a VM's state that up writes, besides state's own.
const (
	KeyNet      = "net"       // network policy: off (on comes with the proxy)
	KeyCPUs     = "cpus"      // the size asked for; unset for the image's
	KeyMemory   = "memory"    // GiB
	KeyDisk     = "disk"      // GiB
	KeyPorts    = "ports"     // which ports it forwards (ParsePorts); unset for all
	KeyPortsSet = "ports_set" // what its backend config forwards; unset for all
)

// Options is what up was asked for.
type Options struct {
	Image           string // the image a new VM gets, as given; "" to detect it
	Backend         string // --backend: a new VM's backend
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
	Dir      string // the folder devvm runs in, a physical path
	Home     string
	OS       string // the host's, as Go names it
	Host     func() (Host, error)
	// Protector returns the .git protection for a project folder.
	Protector func(dir string) *gitprotect.Protector
	// Exec runs the shell in devvm's place; it returns only on failure.
	Exec func(backend.Command) error
}

// NewEnv returns the environment up runs in here.
func NewEnv(d paths.Dirs, root string, reg *backend.Registry) (*Env, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if wd, err = filepath.EvalSymlinks(wd); err != nil {
		return nil, err
	}
	return &Env{
		Dirs: d, Root: root, Registry: reg, Dir: wd, Home: os.Getenv("HOME"), OS: runtime.GOOS,
		Host: ThisHost, Protector: gitprotect.New, Exec: execShell,
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
	builder := &build.Builder{Dirs: e.Dirs, Root: e.Root, Backend: b}
	img, err := e.resolveImage(o.Image, f.Dir, builder)
	if err != nil {
		return err
	}
	if !o.NoNetworkPolicy {
		return msg.NetworkPolicyUnavailable(vm)
	}
	spec := backend.VMSpec{Name: vm, Project: f.Dir, Size: size.Backend(), Ports: PortsBackend(ports)}
	conf := backend.Confinement{Net: backend.NetDirect}
	if err := backend.Require(b, vm, append(spec.Features(), conf.Features()...)...); err != nil {
		return err
	}
	if err := builder.Build(ctx, img); err != nil {
		return err
	}
	chain, err := builder.Inputs().Chain(img)
	if err != nil {
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
		_ = os.Remove(e.Dirs.VMState(vm))
	})
	// The state first, with no network policy said: if a later step
	// fails, the VM never starts without it.
	st, err := state.CreateVM(e.Dirs, vm, state.NewVM{
		Image: img, Dir: f.Dir, Folder: id, Chain: chain.String(), Backend: b.Name(),
	})
	if err != nil {
		return err
	}
	for _, kv := range [][2]string{
		{KeyNet, "off"}, {KeyCPUs, size.CPUs}, {KeyMemory, size.Memory}, {KeyDisk, size.Disk},
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
	if err := b.Start(ctx, vm, conf); err != nil {
		return err
	}
	cmd, err := b.Shell(ctx, vm, f.Dir)
	if err != nil {
		return err
	}
	done()
	return e.Exec(cmd)
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
// one named, or the folder's toolchains when none is.
func (e *Env) resolveImage(want, dir string, b *build.Builder) (string, error) {
	if want == "" {
		found := project.Detect(dir)
		if len(found) == 0 {
			return "", msg.NoImageFor(dir, strings.Join(b.Layers(), " "))
		}
		want = strings.Join(found, "+")
		ui.Info(msg.Detected(want))
	}
	img := build.Normalize(want)
	return img, b.CheckLayers(img)
}
