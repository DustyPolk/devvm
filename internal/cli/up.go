package cli

import (
	"context"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// upFlags are devvm's flags, all of them for what a new VM gets.
type upFlags struct {
	tools, agents   string // --tools and --agents: lists, or none
	layers          string // --layers: a list, or none
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

// addUpFlags adds devvm's flags to the root command, as its own: no
// subcommand takes them but devvm setup, whose --agents chooses new VMs'
// default.
func addUpFlags(root *cobra.Command, uf *upFlags) {
	f := root.Flags()
	f.StringVar(&uf.tools, "tools", "", "the `LIST` of toolchains a new VM gets, such as node,python, or none (default: those the folder's files call for)")
	addAgentsFlag(f, &uf.agents, "the `LIST` of coding agents a new VM gets, such as claude,codex, or none (default: devvm setup's)")
	f.StringVar(&uf.layers, "layers", "", "the `LIST` of layers a new VM's image gets, bottom first, such as docker,mine, or none (default: none)")
	addBackendFlag(f, &uf.backend)
	addFromFlag(f, &uf.from)
	f.BoolVar(&uf.networkPolicy, "network-policy", false, "create the VM with network policy (default: devvm network -g, else asked in a terminal, and on without one)")
	f.BoolVar(&uf.noNetworkPolicy, "no-network-policy", false, "create the VM without network policy")
	exclusive(root, "network-policy", "no-network-policy")
	f.StringVar(&uf.size.CPUs, "cpus", "", "the `N` CPUs the VM gets (default: devvm size -g, else the image's)")
	f.StringVar(&uf.size.Memory, "memory", "", "the VM's memory in `GiB`, such as 16 or 16G (default: devvm size -g, else the image's)")
	f.StringVar(&uf.size.Disk, "disk", "", "the VM's disk in `GiB`, at least the image's (default: devvm size -g, else the image's)")
	f.StringVar(&uf.ports, "ports", "", "the `PORTS` the VM forwards: all, none or a list like 5173,8000-8010 (default: devvm ports -g, else all)")
	f.BoolVar(&uf.clone, "clone", false, "create the VM with its own clone of this repo instead of mounting the folder")
	f.BoolVar(&uf.unsafeFolder, "unsafe-folder", false, "mount a folder devvm refuses, such as your home folder")
	f.BoolVar(&uf.noGitProtect, "no-git-protect", false, "create the VM without .git protection")
	f.BoolVar(&uf.sshAgent, "ssh-agent", false, "create the VM with your SSH agent forwarded, so anything in it can use your keys while you're connected")
	f.BoolVar(&uf.noDevcontainer, "no-devcontainer", false, "create the VM without reading the folder's devcontainer.json")
	f.BoolVar(&uf.here, "here", false, "create a VM for this folder even when a folder it's inside has one")
	addSSHConfigFlag(f, &uf.sshConfig)
}

// options are what the flags ask of up.
func (uf *upFlags) options() vm.Options {
	return vm.Options{
		Toolchains: partFlag(uf.tools), Agents: partFlag(uf.agents), Layers: partFlag(uf.layers),
		Backend: uf.backend, NetworkPolicy: uf.networkPolicy, NoNetworkPolicy: uf.noNetworkPolicy, Size: uf.size, Ports: uf.ports,
		From: fromRef(uf.from), FromSet: uf.from != "",
		UnsafeFolder: uf.unsafeFolder, NoGitProtect: uf.noGitProtect, Clone: uf.clone, SSHAgent: uf.sshAgent,
		NoDevcontainer: uf.noDevcontainer, Here: uf.here,
	}
}

// up opens a shell in this folder's VM, making it first.
func up(ctx context.Context, u *ui.UI, uf *upFlags) error {
	o := uf.options()
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
	o := readOverrides(os.Getenv)
	reg, err := registry.New(dirs, assets.FS, u, o.Overrides)
	if err != nil {
		return nil, err
	}
	e, err := vm.NewEnv(dirs, assets.FS, self, reg, u)
	if err != nil {
		return nil, err
	}
	e.Tools.Mise = o.HostMise
	e.PickFeatures = pickFeatures(u)
	e.PickParts = pickParts(u)
	return e, nil
}

// pickParts returns what shows the tools picker in u's terminal, each
// part under its heading; without one, the rows stay as they are.
func pickParts(u *ui.UI) func(msg.M, []vm.PickPart) []vm.PickPart {
	return func(help msg.M, parts []vm.PickPart) []vm.PickPart {
		sections := make([]ui.Section, len(parts))
		for i, p := range parts {
			sections[i].Heading = p.Heading
			for _, r := range p.Rows {
				sections[i].Choices = append(sections[i].Choices, ui.Choice{Name: r.Name, Desc: r.Desc, On: r.On, Fixed: r.Fixed})
			}
		}
		picked := u.Pick(help, sections)
		out := make([]vm.PickPart, len(picked))
		for i, s := range picked {
			out[i].Heading = s.Heading
			for _, c := range s.Choices {
				out[i].Rows = append(out[i].Rows, vm.PickRow{Name: c.Name, Desc: c.Desc, On: c.On, Fixed: c.Fixed})
			}
		}
		return out
	}
}

// partsNone is the value of --tools, --agents and --layers that chooses
// none of the part.
const partsNone = "none"

// partFlag is a part's flag as up takes it: nil when it's not given, its
// list otherwise, empty for none.
func partFlag(value string) *[]string {
	switch value {
	case "":
		return nil
	case partsNone:
		return &[]string{}
	}
	names := splitList(value)
	return &names
}

// checkPartFlag checks the value of --tools, --agents or --layers (flag,
// as given): none alone, or a comma-separated list of the part's names,
// each refused with a did-you-mean when it isn't one. The layers are base
// and devvm's own, and the user's.
func checkPartFlag(flag, value string) error {
	names := splitList(value)
	if slices.Contains(names, partsNone) && len(names) > 1 || len(names) == 0 {
		return msg.PartNoneAlone(flag)
	}
	if value == partsNone {
		return nil
	}
	switch flag {
	case "--tools":
		return vm.CheckPart(flag, names, image.Toolchains)
	case "--agents":
		return vm.CheckAgents(names)
	}
	layers := image.Builtins
	if dirs, err := paths.FromEnv(); err == nil {
		layers = (&build.Env{Dirs: dirs}).Layers()[1:]
	}
	return vm.CheckPart(flag, names, layers)
}
