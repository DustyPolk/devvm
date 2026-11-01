package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newSizeCmd is devvm size [--cpus N] [--memory GiB] [--disk GiB], with -g
// for the size new VMs get.
func newSizeCmd(u *ui.UI) *cobra.Command {
	var (
		global, asJSON bool
		s              vm.Size
	)
	cmd := &cobra.Command{
		Use:   "size",
		Short: "Show or change this folder's VM's size",
		Long: "Show this folder's VM's CPUs, memory and disk, or change them with --cpus, --memory and --disk. " +
			"A stopped VM changes at once, and a running VM changes when it next starts, such as with devvm restart. " +
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
			withEnv(u, func(env *vm.Env) error {
				set := vm.Settings{Env: env}
				if s != (vm.Size{}) {
					return set.SetSize(cmd.Context(), s, global)
				}
				var err error
				shown, err = set.ShowSize(cmd.Context(), global)
				return err
			}, msg.SizeFailed)
			switch {
			case s != (vm.Size{}):
				return nil
			case asJSON:
				return printJSON(u, sizeJSON(shown))
			}
			_, err := io.WriteString(u.Stdout(), shown.Text())
			return err
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&global, "global", "g", false, "show or change the size new VMs get instead")
	f.StringVar(&s.CPUs, "cpus", "", "the `N` CPUs the VM gets")
	f.StringVar(&s.Memory, "memory", "", "the VM's memory in `GiB`, such as 16 or 16G")
	f.StringVar(&s.Disk, "disk", "", "the VM's disk in `GiB`, at least what it has")
	jsonFlag(cmd, &asJSON)
	return cmd
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

// sizeOutput is devvm size --json: the VM's size, what its disk holds on
// a backend that sizes no disks, and what changes at its next start; with
// -g, the size new VMs get.
type sizeOutput struct {
	sizeShape
	DiskUsedGiB *float64   `json:"disk_used_gib,omitempty"`
	Pending     *sizeShape `json:"pending,omitempty"`
}

func sizeJSON(s vm.SizeShown) sizeOutput {
	out := sizeOutput{sizeShape: shapeOfSize(s.Size)}
	if s.DiskMeasured {
		out.DiskUsedGiB = &s.DiskUsedGiB
	}
	if s.Pending != (backend.Size{}) {
		p := shapeOfSize(s.Pending)
		out.Pending = &p
	}
	return out
}
