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
		Short: "Run Lima's limactl on devvm's own Lima home",
		Long: "Run limactl with ARGS on devvm's own Lima home, where the lima backend's VMs and images live, " +
			"in devvm's place: its input, output and exit code are limactl's, and every argument is passed " +
			"on, flags and --help included. It runs in devvm's data folder, " +
			"so give it files by absolute path. A VM started this way runs unconfined, without its " +
			"network policy (devvm ls shows it unenforced): start VMs with devvm.",
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
