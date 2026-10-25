package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// globals are the flags every command takes.
type globals struct {
	backend         string
	networkPolicy   bool
	noNetworkPolicy bool
	size            vm.Size
	ports           string
	from            string
}

// fromRef is --from, else its default from the environment.
func (g *globals) fromRef() string {
	if g.from != "" {
		return g.from
	}
	return os.Getenv(from.Env)
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
	f.StringVar(&g.from, "from", "", "the container `IMAGE` a new VM's or image's base starts from instead of Ubuntu's cloud image: node:24, debian:13, ghcr.io/org/image@sha256:… (default: $DEVVM_FROM's)")
	f.StringVar(&g.ports, "ports", "", "which `PORTS` a new VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: the ports file's, else all)")
}

// upFlags are the flags only up takes.
type upFlags struct {
	unsafeFolder bool
	noGitProtect bool
	clone        bool
	sshAgent     bool
}

func addUpFlags(cmd *cobra.Command, uf *upFlags) {
	f := cmd.Flags()
	f.BoolVar(&uf.clone, "clone", false, "create the VM with its own clone of this repo instead of mounting the folder: your branches are copied in, and the repo gets a devvm remote to fetch its work from")
	f.BoolVar(&uf.unsafeFolder, "unsafe-folder", false, "mount a folder that is or holds your home folder or devvm's own, or is inside devvm's own, anyway")
	f.BoolVar(&uf.noGitProtect, "no-git-protect", false, "create the VM without .git protection")
	f.BoolVar(&uf.sshAgent, "ssh-agent", false, "create the VM with your SSH agent forwarded: while you're connected, anything in it can use your keys (devvm ssh-agent changes it later)")
}

// options are what the global flags ask of up.
func (g *globals) options() vm.Options {
	return vm.Options{
		Backend: g.backend, NetworkPolicy: g.networkPolicy, NoNetworkPolicy: g.noNetworkPolicy, Size: g.size, Ports: g.ports,
		From: g.fromRef(),
	}
}

// up opens a shell in this folder's VM, making it first.
func up(ctx context.Context, u *ui.UI, g *globals, uf *upFlags, args []string) error {
	o := g.options()
	o.UnsafeFolder, o.NoGitProtect, o.Clone, o.SSHAgent = uf.unsafeFolder, uf.noGitProtect, uf.clone, uf.sshAgent
	if len(args) > 0 {
		o.Image = args[0]
	}
	withEnv(u, func(env *vm.Env) error { return vm.Launch{Env: env}.Up(ctx, o) }, msg.UpFailed)
	return nil
}

// withEnv runs do in this folder's environment on u, failing with what's
// message for an error that isn't one.
func withEnv(u *ui.UI, do func(*vm.Env) error, what func(reason string) msg.M) {
	env, err := newEnv(u)
	if err == nil {
		err = do(env)
	}
	fail(u, err, what)
}

func newEnv(u *ui.UI) (*vm.Env, error) {
	dirs, err := paths.FromEnv()
	if err != nil {
		return nil, err
	}
	self, err := paths.Self()
	if err != nil {
		return nil, err
	}
	reg, err := backends.Registry(dirs, u)
	if err != nil {
		return nil, err
	}
	e, err := vm.NewEnv(dirs, filepath.Dir(self), self, reg, u)
	if err != nil {
		return nil, err
	}
	e.PickFeatures = pickFeatures(u)
	return e, nil
}
