package cli

import (
	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/vm"
)

// newMountCacheCmd is devvm mount-cache [none]: none is the one mode devvm
// sets, and the value stays a word, so the command reads as what it shows
// and stays the same should another mode be worth setting one day.
func newMountCacheCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:         "mount-cache [none]",
		Annotations: about{docs: "backends/lima#the-project-folders-cache", group: groupChange}.annotations(),
		Short:       "Show or change how this folder's VM caches its project folder",
		Long: "Show how this folder's VM caches its project folder on a Linux host: none, which new VMs get, " +
			"or mmap for a VM made before devvm chose none, which can lose writes and breaks SQLite and Gradle there. " +
			"devvm mount-cache none switches it from the VM's next start, so in a terminal devvm offers to restart a running VM now.",
		Example: examples(
			"devvm mount-cache", "show the VM's cache mode",
			"devvm mount-cache none", "stop the VM losing writes, from its next start",
		),
		Args: showsOnly(cobra.MatchAll(maxArgs(1), func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return nil
			}
			return vm.CheckCacheMode(args[0])
		})),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return backend.CacheModes(), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var shown vm.MountCacheShown
			err := a.env(func(env *vm.Env) error {
				s := vm.Settings{Env: env}
				if len(args) == 1 {
					return s.SetMountCache(cmd.Context(), args[0])
				}
				var err error
				shown, err = s.MountCache(cmd.Context())
				return err
			}, msg.MountCacheFailed)
			switch {
			case err != nil || len(args) == 1:
				return err
			case asJSON:
				return printJSON(a.u, mountCacheOutput{Mode: shown.Mode, Chosen: shown.Chosen, Pending: shown.Pending})
			}
			return printValue(a.u, false, "mode", shown.Mode)
		},
	}
	jsonFlag(cmd, &asJSON)
	return addFolderFlags(cmd, a)
}

// mountCacheOutput is devvm mount-cache --json: the VM's mode, whether it
// was chosen rather than kept from its backend, and the mode its next
// start gives it.
type mountCacheOutput struct {
	Mode    string `json:"mode"`
	Chosen  bool   `json:"chosen"`
	Pending string `json:"pending,omitempty"`
}
