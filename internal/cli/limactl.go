package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// newLimactlCmd is devvm limactl: Lima's own command on the lima
// backend's VMs and images.
func newLimactlCmd(u *ui.UI) *cobra.Command {
	c := &cobra.Command{
		Use:   "limactl [ARGS...]",
		Short: "Run Lima's limactl for debugging, which starts VMs without network policy",
		Long: "Run limactl with ARGS on devvm's own Lima home, where the lima backend's VMs and images live, " +
			"passing on every argument, --help included. " +
			"A VM started this way runs without its network policy, so start VMs with devvm.",
		Example: examples(
			"devvm limactl list", "list the lima backend's VMs and images",
			"devvm limactl shell devvm-website-71a99b1076d92c", "open a shell in a running VM through Lima",
		),
		DisableFlagParsing:    true,
		DisableFlagsInUseLine: true,
		RunE: func(_ *cobra.Command, args []string) error {
			d, err := paths.FromEnv()
			if err == nil {
				err = lima.Limactl(d, args)
			}
			fail(u, err, msg.LimactlFailed)
			return nil
		},
	}
	// --help is limactl's: hidden from devvm's help for this command.
	c.Flags().BoolP("help", "h", false, "")
	_ = c.Flags().MarkHidden("help")
	return c
}
