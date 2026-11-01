package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// upFlags are devvm [IMAGE]'s flags, all of them for what a new VM gets.
type upFlags struct {
	backend         string
	from            string
	networkPolicy   bool
	noNetworkPolicy bool
	size            vm.Size
	ports           string
	unsafeFolder    bool
	noGitProtect    bool
	clone           bool
	sshAgent        bool
	noDevcontainer  bool
	here            bool
	sshConfig       bool
}

// addUpFlags adds devvm [IMAGE]'s flags to the root command, as its own:
// no subcommand takes them.
func addUpFlags(root *cobra.Command, uf *upFlags) {
	f := root.Flags()
	addBackendFlag(f, &uf.backend)
	addFromFlag(f, &uf.from)
	f.BoolVar(&uf.networkPolicy, "network-policy", false, "create the VM with network policy: it reaches only the hosts you allow, through a proxy on this machine, and your secrets stay on this machine (default: devvm network -g, else asked in a terminal, and on without one)")
	f.BoolVar(&uf.noNetworkPolicy, "no-network-policy", false, "create the VM without network policy: it reaches anything this machine can")
	exclusive(root, "network-policy", "no-network-policy")
	f.StringVar(&uf.size.CPUs, "cpus", "", "the `N` CPUs the VM gets (default: devvm size -g, else the image's)")
	f.StringVar(&uf.size.Memory, "memory", "", "the VM's memory in `GiB`: 16, 16G or 16GiB (default: devvm size -g, else the image's)")
	f.StringVar(&uf.size.Disk, "disk", "", "the VM's disk in `GiB`, at least the image's (default: devvm size -g, else the image's)")
	f.StringVar(&uf.ports, "ports", "", "which `PORTS` the VM forwards to this machine's localhost: all, none, or a list like 5173,8000-8010 (default: devvm ports -g, else all)")
	f.BoolVar(&uf.clone, "clone", false, "create the VM with its own clone of this repo instead of mounting the folder: your branches are copied in, and the repo gets a devvm remote to fetch its work from")
	f.BoolVar(&uf.unsafeFolder, "unsafe-folder", false, "mount a folder that is or holds your home folder or devvm's own, or is inside devvm's own, anyway")
	f.BoolVar(&uf.noGitProtect, "no-git-protect", false, "create the VM without .git protection")
	f.BoolVar(&uf.sshAgent, "ssh-agent", false, "create the VM with your SSH agent forwarded: while you're connected, anything in it can use your keys (devvm ssh-agent changes it later)")
	f.BoolVar(&uf.noDevcontainer, "no-devcontainer", false, "create the VM without reading the folder's .devcontainer/devcontainer.json or .devcontainer.json")
	f.BoolVar(&uf.here, "here", false, "create a VM for this folder when it has none, even when a folder it's inside has one, without asking whether to use that VM")
	addSSHConfigFlag(f, &uf.sshConfig)
}

// options are what the flags ask of up.
func (uf *upFlags) options() vm.Options {
	return vm.Options{
		Backend: uf.backend, NetworkPolicy: uf.networkPolicy, NoNetworkPolicy: uf.noNetworkPolicy, Size: uf.size, Ports: uf.ports,
		From: fromRef(uf.from), FromSet: uf.from != "",
		UnsafeFolder: uf.unsafeFolder, NoGitProtect: uf.noGitProtect, Clone: uf.clone, SSHAgent: uf.sshAgent,
		NoDevcontainer: uf.noDevcontainer, Here: uf.here,
	}
}

// up opens a shell in this folder's VM, making it first.
func up(ctx context.Context, u *ui.UI, uf *upFlags, args []string) error {
	o := uf.options()
	if len(args) > 0 {
		o.Image = args[0]
	}
	withEnv(u, func(env *vm.Env) error {
		env.AddInclude = uf.sshConfig
		return vm.Launch{Env: env}.Up(ctx, o)
	}, msg.UpFailed)
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
	reg, err := registry.New(dirs, assets.FS, u)
	if err != nil {
		return nil, err
	}
	e, err := vm.NewEnv(dirs, assets.FS, self, reg, u)
	if err != nil {
		return nil, err
	}
	e.PickFeatures = pickFeatures(u)
	return e, nil
}
