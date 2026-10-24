package cli

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/vm"
)

// globals are the flags every command takes.
type globals struct {
	backend         string
	networkPolicy   bool
	noNetworkPolicy bool
	size            vm.Size
	ports           string
}

// addGlobals adds the global flags to the root command.
func addGlobals(root *cobra.Command, g *globals) {
	f := root.PersistentFlags()
	f.StringVar(&g.backend, "backend", "", "the `NAME` of the backend a new VM or image gets (default: the default backend file's, else lima)")
	f.BoolVar(&g.networkPolicy, "network-policy", false, "create the VM with network policy: it reaches only an allowlist of hosts, through a proxy on this machine, and your tokens stay here (default: ask in a terminal, else off)")
	f.BoolVar(&g.noNetworkPolicy, "no-network-policy", false, "create the VM without network policy: it reaches anything this machine can")
	root.MarkFlagsMutuallyExclusive("network-policy", "no-network-policy")
	f.StringVar(&g.size.CPUs, "cpus", "", "the `N` CPUs a new VM gets (default: the size file's, else the image's)")
	f.StringVar(&g.size.Memory, "memory", "", "a new VM's memory in `GiB`: 16, 16G or 16GiB (default: the size file's, else the image's)")
	f.StringVar(&g.size.Disk, "disk", "", "a new VM's disk in `GiB`, at least the image's (default: the size file's, else the image's)")
	f.StringVar(&g.ports, "ports", "", "which `PORTS` a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)")
}

// upFlags are the flags only up takes.
type upFlags struct {
	unsafeFolder bool
	noGitProtect bool
}

func addUpFlags(cmd *cobra.Command, u *upFlags) {
	f := cmd.Flags()
	f.BoolVar(&u.unsafeFolder, "unsafe-folder", false, "mount a folder that is or holds your home folder or devvm's own, or is inside devvm's own, anyway")
	f.BoolVar(&u.noGitProtect, "no-git-protect", false, "create the VM without .git protection")
}

// options are what the global flags ask of up.
func (g *globals) options() vm.Options {
	return vm.Options{
		Backend: g.backend, NetworkPolicy: g.networkPolicy, NoNetworkPolicy: g.noNetworkPolicy, Size: g.size, Ports: g.ports,
	}
}

// up opens a shell in this folder's VM, making it first.
func up(ctx context.Context, g *globals, u *upFlags, args []string) error {
	o := g.options()
	o.UnsafeFolder, o.NoGitProtect = u.unsafeFolder, u.noGitProtect
	if len(args) > 0 {
		o.Image = args[0]
	}
	withEnv(func(env *vm.Env) error { return env.Up(ctx, o) }, msg.UpFailed)
	return nil
}

// withEnv runs do in this folder's environment, failing with what's
// message for an error that isn't one.
func withEnv(do func(*vm.Env) error, what func(reason string) msg.M) {
	env, err := newEnv()
	if err == nil {
		err = do(env)
	}
	fail(err, what)
}

func newEnv() (*vm.Env, error) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, err
	}
	self, err := paths.Self()
	if err != nil {
		return nil, err
	}
	reg, err := backends.Registry(dirs)
	if err != nil {
		return nil, err
	}
	return vm.NewEnv(dirs, filepath.Dir(self), self, reg)
}
