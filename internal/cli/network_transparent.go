package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/netpolicy"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/ui"
	"github.com/devvmhq/devvm/internal/vm"
)

// newNetworkTransparentCmd is devvm network transparent [on|off].
func newNetworkTransparentCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:         "transparent [on|off]",
		Annotations: about{docs: "safety/network#tools-that-ignore-the-proxy"}.annotations(),
		Short:       "Show or change transparent mode for this folder's VM",
		Long: "Show whether this folder's VM sends tools that ignore the proxy settings to its proxy by the names they look up, on or off, and where that comes from: the VM's own setting, config.toml's transparent-proxy, or devvm's default, on. " +
			"devvm network transparent on or off gives the VM its own setting, which applies from its next launch, while it's on a confined tier.",
		Example: examples(
			"devvm network transparent", "show the VM's transparent mode and where it comes from",
			"devvm network transparent off", "turn it off for this VM, from its next launch",
			"devvm network transparent on", "turn it on again, whatever config.toml says",
		),
		Args:      showsOnly(maxArgs(1)),
		ValidArgs: netpolicy.TransparentChoices(),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				t    netpolicy.Transparent
				dirs paths.Dirs
			)
			err := a.env(func(env *vm.Env) error {
				n := vm.Settings{Env: env}
				dirs = env.Dirs
				if len(args) == 1 {
					return n.SetTransparent(cmd.Context(), args[0], "devvm network transparent "+args[0])
				}
				var err error
				t, err = n.Transparent(cmd.Context())
				return err
			}, msg.NetTransparentFailed)
			if err != nil || len(args) == 1 {
				return err
			}
			return printTransparent(a.u, asJSON, t, dirs)
		},
	}
	jsonFlag(cmd, &asJSON)
	return addFolderFlags(cmd, a)
}

// transparentJSON is a VM's transparent mode, in devvm network transparent
// --json and devvm network --json: whether it's on, and where that comes
// from, vm, config or default.
type transparentJSON struct {
	On   bool   `json:"on"`
	From string `json:"from"`
}

// printTransparent writes a VM's transparent mode, on or off, then a #
// and where it comes from (transparentFrom), or with asJSON as
// transparentJSON.
func printTransparent(u *ui.UI, asJSON bool, t netpolicy.Transparent, d paths.Dirs) error {
	if asJSON {
		return printJSON(u, transparentJSON(t))
	}
	v := netpolicy.TransparentOff
	if t.On {
		v = netpolicy.TransparentOn
	}
	_, err := io.WriteString(u.Stdout(), v+" # "+transparentFrom(t.From, d)+"\n")
	return err
}

// transparentFrom is where a VM's transparent mode comes from, as devvm
// network transparent shows it: the command that set it, config.toml's
// path, or devvm's default.
func transparentFrom(from string, d paths.Dirs) string {
	switch from {
	case netpolicy.FromVM:
		return "devvm network transparent"
	case netpolicy.FromConfig:
		return d.ConfigFile()
	}
	return "devvm: default"
}
