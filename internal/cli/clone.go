package cli

import (
	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/vm"
)

// newCloneCmd is devvm clone [on|off] [--force] [--no-export]
// [--clean-home] [--no-clone-check].
func newCloneCmd(a *app) *cobra.Command {
	var o vm.RemakeOptions
	var asJSON bool
	cmd := &cobra.Command{
		Use:         "clone [on|off]",
		Annotations: about{docs: "using/clone#switching-a-vm-that-exists", group: groupChange}.annotations(),
		Short:       "Show or change whether this folder's VM has its own clone of the repo",
		Long: "Show whether this folder's VM has its own clone of the repo, on, or mounts this folder, off. " +
			"devvm clone on and devvm clone off make the VM again from its image, after asking in a terminal without --no-input, then open a shell in it, and devvm keeps its home folder and its settings, while everything else on its disk is lost. " +
			"On copies the repo's branches into the new clone, and off first offers to fetch the clone's work into this repo, unless you give --no-clone-check.",
		Example: examples(
			"devvm clone", "show whether the VM has its own clone of the repo",
			"devvm clone on", "make the VM again with its own clone of the repo, after asking",
			"devvm clone off", "fetch the clone's work, then make the VM again with this folder mounted",
			"devvm clone on --force", "make it again without asking, as a script must",
		),
		Args:      showsOnly(maxArgs(1), "force", "no-export", "clean-home", "no-clone-check"),
		ValidArgs: []string{vm.CloneOn, vm.CloneOff},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				command := "devvm clone " + args[0]
				return a.env(func(env *vm.Env) error {
					return vm.Lifecycle{Env: env}.SetClone(cmd.Context(), command, args[0], o)
				}, msg.RemakeFailed)
			}
			var mode string
			if err := a.env(func(env *vm.Env) error {
				var err error
				mode, err = vm.Settings{Env: env}.Clone(cmd.Context())
				return err
			}, msg.CloneShowFailed); err != nil {
				return err
			}
			return printValue(a.u, asJSON, "clone", mode)
		},
	}
	addRemakeFlags(cmd.Flags(), &o)
	addNoCloneCheckFlag(cmd.Flags(), &o.NoCloneCheck)
	jsonFlag(cmd, &asJSON)
	return addFolderFlags(cmd, a)
}
