package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newDesktopCmd is devvm desktop.
func newDesktopCmd(u *ui.UI) *cobra.Command {
	var o vm.DesktopOptions
	c := &cobra.Command{
		Use:   "desktop",
		Short: "Open this folder's VM's desktop in a VNC viewer",
		Long: "Get this folder's VM ready as devvm ssh does, start the desktop of its desktop feature, and " +
			"serve it on this machine's 127.0.0.1 alone, with a password of the VM's own, until you press " +
			"Ctrl+C or the VM stops. On macOS, Screen Sharing opens it and asks for the password, which devvm " +
			"shows. The clipboard isn't shared unless you add --clipboard.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Desktop{Env: env}.Open(cmd.Context(), o) }, msg.DesktopFailed)
			return nil
		},
	}
	f := c.Flags()
	f.BoolVar(&o.Clipboard, "clipboard", false, "share the clipboard both ways: this machine's reaches the VM, and the VM's this machine")
	f.BoolVar(&o.NoOpen, "no-open", false, "show the address and password without opening Screen Sharing")
	return c
}
