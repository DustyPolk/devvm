package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/vm"
)

// newSizeCmd is devvm size [--cpus N] [--memory GiB] [--disk GiB], with -g
// for the size new VMs get.
func newSizeCmd(a *app) *cobra.Command {
	u := a.u
	var (
		global, asJSON bool
		s              vm.Size
	)
	cmd := &cobra.Command{
		Use:         "size",
		Annotations: about{docs: "using/ports-size#size", group: groupChange}.annotations(),
		Short:       "Show or change this folder's VM's size",
		Long: "Show this folder's VM's CPUs, memory and disk, or change them with --cpus, --memory and --disk. " +
			"The VM changes when it next starts, so for a running VM devvm offers in a terminal to restart it now. " +
			"With -g, show or change the size new VMs get instead.",
		Example: examples(
			"devvm size", "show the VM's size",
			"devvm size --cpus 6 --memory 16", "give it 6 CPUs and 16 GiB of memory",
			"devvm size --disk 200", "grow its disk to 200 GiB",
			"devvm size -g --memory 16", "give new VMs 16 GiB of memory",
		),
		Args: showsOnly(noArgs, "cpus", "memory", "disk"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			var shown vm.SizeShown
			err := a.env(func(env *vm.Env) error {
				set := vm.Settings{Env: env}
				if s != (vm.Size{}) {
					return set.SetSize(cmd.Context(), s, global)
				}
				var err error
				shown, err = set.ShowSize(cmd.Context(), global)
				return err
			}, msg.SizeFailed)
			switch {
			case err != nil || s != (vm.Size{}):
				return err
			case asJSON:
				return printJSON(u, sizeJSON(shown))
			}
			_, err = io.WriteString(u.Stdout(), shown.Text())
			return err
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&global, "global", "g", false, "show or change the size new VMs get instead")
	f.StringVar(&s.CPUs, "cpus", "", "the `N` CPUs the VM gets")
	f.StringVar(&s.Memory, "memory", "", "the VM's memory in `GiB`, such as 16 or 16G")
	f.StringVar(&s.Disk, "disk", "", "the VM's disk in `GiB`, at least what it has")
	jsonFlag(cmd, &asJSON)
	return addFolderFlags(cmd, a)
}

// sizeShape is a size in devvm size --json: what's unset (the image's,
// for -g; a disk on a backend that sizes none) is left out.
type sizeShape struct {
	CPUs      int     `json:"cpus,omitempty"`
	MemoryGiB float64 `json:"memory_gib,omitempty"`
	DiskGiB   float64 `json:"disk_gib,omitempty"`
}

func shapeOfSize(s backend.Size) sizeShape {
	return sizeShape{CPUs: s.CPUs, MemoryGiB: s.MemoryGiB, DiskGiB: s.DiskGiB}
}

// sizeOutput is devvm size --json: the VM's size and what changes at its
// next start; with -g, the size new VMs get, and sources, where each of
// cpus, memory and disk came from (a settings.Source).
type sizeOutput struct {
	sizeShape
	Pending *sizeShape        `json:"pending,omitempty"`
	Sources map[string]string `json:"sources,omitempty"`
}

func sizeJSON(s vm.SizeShown) sizeOutput {
	out := sizeOutput{sizeShape: shapeOfSize(s.Size), Sources: s.Sources}
	if s.Pending != (backend.Size{}) {
		p := shapeOfSize(s.Pending)
		out.Pending = &p
	}
	return out
}
