package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newLinksCmd is devvm _links VM: the link session of a running client
// VM, which carries its links to other VMs' ports until the VM stops or
// has none (vm.Links.Session). devvm starts it detached; its log lines are
// data, written to devvm's stderr. Hidden: it's not for users.
func newLinksCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:    "_links VM",
		Short:  "Carry a VM's links to other VMs",
		Hidden: true,
		Args:   exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a.env(func(env *vm.Env) error {
				return vm.Links{Env: env}.Session(cmd.Context(), args[0], a.u.Stderr())
			}, msg.LinkSessionFailed)
			return nil
		},
	}
}
