package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSizeCmd is devvm size [--cpus N] [--memory GiB] [--disk GiB], with -g
// for the size new VMs get. Its size flags are its own, in place of the
// root's for new VMs.
func newSizeCmd() *cobra.Command {
	var (
		global bool
		s      vm.Size
	)
	cmd := &cobra.Command{
		Use:   "size",
		Short: "Show or change this folder's VM's size",
		Long: "Show this folder's VM's CPUs, memory and disk, and what changes when it next starts. " +
			"--cpus, --memory and --disk change them, at once for a stopped VM, from its next start for " +
			"a running one: devvm restart. A disk only grows. With -g, show or change the size new VMs " +
			"get instead; what it leaves unset is the image's.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(func(env *vm.Env) error {
				if s == (vm.Size{}) {
					return env.ShowSize(cmd.Context(), ui.Stdout(), global)
				}
				return env.SetSize(cmd.Context(), s, global)
			}, msg.SizeFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&global, "global", "g", false, "show or change the size new VMs get instead")
	f.StringVar(&s.CPUs, "cpus", "", "the `N` CPUs the VM gets")
	f.StringVar(&s.Memory, "memory", "", "the VM's memory in `GiB`: 16, 16G or 16GiB")
	f.StringVar(&s.Disk, "disk", "", "the VM's disk in `GiB`, at least what it has")
	return cmd
}
