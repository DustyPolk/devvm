package cli

import (
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/vmssh"
)

// newForwarderCmd is devvm _ports: a VM's port forwarder (vmssh.Main),
// on every backend, which devvm starts detached (vmssh.Forwards). It
// gets ports.sh and the ports devvm records for VMs, which it never
// forwards (ports.Used) but its VM's own listed ones; its log lines are
// data, written to devvm's stderr. Hidden: it's not for users. Its flags
// are the forwarder's own, parsed by vmssh.Main.
func newForwarderCmd(a *app) *cobra.Command {
	u := a.u
	return &cobra.Command{
		Use:                vmssh.Cmd + " [flags]",
		Short:              "Forward a VM's ports",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fail := func(err error) error { return msg.ForwarderFailed(err.Error()).Because(err) }
			d, err := a.dirs()
			if err != nil {
				return fail(err)
			}
			script, err := fs.ReadFile(assets.FS, "guest/ports.sh")
			if err != nil {
				return fail(err)
			}
			reserved := func(vm string) (map[int]bool, error) { return ports.Used(d, vm) }
			if err := vmssh.Main(cmd.Context(), args, script, reserved, u.Stderr()); err != nil {
				return fail(err)
			}
			return nil
		},
	}
}
