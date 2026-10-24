package cli

import (
	"context"

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
	f.BoolVar(&g.networkPolicy, "network-policy", false, "create the VM with network policy (not available yet: refused)")
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

// up opens a shell in this folder's VM, making it first.
func up(ctx context.Context, g *globals, u *upFlags, args []string) error {
	o := vm.Options{
		Backend: g.backend, NoNetworkPolicy: g.noNetworkPolicy, Size: g.size, Ports: g.ports,
		UnsafeFolder: u.unsafeFolder, NoGitProtect: u.noGitProtect,
	}
	if len(args) > 0 {
		o.Image = args[0]
	}
	dirs, err := paths.FromEnv()
	if err == nil {
		var root string
		if root, err = paths.Root(); err == nil {
			err = runUp(ctx, dirs, root, o)
		}
	}
	fail(err, msg.UpFailed)
	return nil
}

func runUp(ctx context.Context, dirs paths.Dirs, root string, o vm.Options) error {
	reg, err := backends.Registry(dirs)
	if err != nil {
		return err
	}
	env, err := vm.NewEnv(dirs, root, reg)
	if err != nil {
		return err
	}
	return env.Up(ctx, o)
}
