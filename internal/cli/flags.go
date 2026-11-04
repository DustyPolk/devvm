package cli

import (
	"os"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/w3cj/devvm/internal/from"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// addFromFlag adds --from, the container image a base starts from.
func addFromFlag(f *pflag.FlagSet, p *string) {
	f.StringVar(p, "from", "", "the container `IMAGE` base starts from, such as node:24, instead of Ubuntu's cloud image (default: $DEVVM_FROM)")
}

// addAgentsFlag adds --agents, a list of coding agents: a new VM's, or
// new VMs' default with devvm setup. usage says whose.
func addAgentsFlag(f *pflag.FlagSet, p *string, usage string) {
	f.StringVar(p, "agents", "", usage)
}

// addLayersFlag adds --layers, a list of layers: a new VM's, or new VMs'
// default with devvm setup. usage says whose.
func addLayersFlag(f *pflag.FlagSet, p *string, usage string) {
	f.StringVar(p, "layers", "", usage)
}

// addForceFlag adds -f/--force, which only skips a confirmation.
func addForceFlag(f *pflag.FlagSet, p *bool) {
	f.BoolVarP(p, "force", "f", false, "don't ask to confirm, which devvm needs without a terminal")
}

// addNoExportFlag adds --no-export, which skips exporting the agents'
// history before a VM's disk goes (devvm rm, devvm layer add, devvm
// layer rm, devvm update).
func addNoExportFlag(f *pflag.FlagSet, p *bool) {
	f.BoolVar(p, "no-export", false, "don't export the agents' history first")
}

// addRemakeFlags adds the flags of a command that makes the folder's VM
// again (devvm layer add, devvm layer rm, devvm update): --force,
// --no-export and
// --clean-home.
func addRemakeFlags(f *pflag.FlagSet, o *vm.RemakeOptions) {
	addForceFlag(f, &o.Force)
	addNoExportFlag(f, &o.NoExport)
	f.BoolVar(&o.CleanHome, "clean-home", false, "give the VM the image's home folder instead of keeping its own")
}

// addSSHConfigFlag adds --ssh-config, which answers the question whether
// to add devvm's Include line to ~/.ssh/config.
func addSSHConfigFlag(f *pflag.FlagSet, p *bool) {
	f.BoolVar(p, "ssh-config", false, "add devvm's Include line to ~/.ssh/config without asking")
}

// confirmed is whether a destructive command (command, as typed) goes
// ahead: with --force without asking; in a terminal, when ask says yes;
// without one, never, refused with force_needed.
func confirmed(u *ui.UI, force bool, command string, ask func() bool) (bool, error) {
	switch {
	case force:
		return true, nil
	case !u.StdinIsTerminal():
		return false, msg.ForceNeeded(command)
	}
	return ask(), nil
}

// fromRef is --from as given, else its default from the environment.
func fromRef(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv(from.Env)
}

// noBlanks refuses a flag given an empty value, and an empty argument: no
// flag or argument takes a blank to mean none or its default. A command
// that passes its arguments on as they are (limactl) is left alone.
func noBlanks(cmd *cobra.Command, args []string) error {
	if cmd.DisableFlagParsing {
		return nil
	}
	var err error
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if err == nil && f.Value.Type() == "string" && f.Value.String() == "" {
			err = msg.FlagBlank(f.Name)
		}
	})
	if err != nil {
		return err
	}
	if slices.Contains(args, "") {
		return msg.ArgBlank(cmd.CommandPath())
	}
	return nil
}
