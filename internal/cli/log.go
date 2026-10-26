package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLogCmd is devvm log [-f] [-n N] [--blocked].
func newLogCmd(u *ui.UI) *cobra.Command {
	var lines uint
	var o vm.LogOptions
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show what this folder's VM connected to and what its network policy blocked",
		Long: "Show the network log of this folder's VM, which has network policy: one line per connection " +
			"or request its proxy allowed or blocked, oldest first, with the time, the host and port, and " +
			"the rule that allowed it, the request and the tokens put in, or why it was blocked. " +
			"--blocked lists the blocks instead, one row per host and port, with the devvm policy allow " +
			"command that allows each.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.Lines = int(min(lines, 1<<31-1))
			withEnv(u, func(env *vm.Env) error { return vm.Net{Env: env}.Log(cmd.Context(), u.Stdout(), o) }, msg.LogFailed)
			return nil
		},
	}
	f := cmd.Flags()
	f.UintVarP(&lines, "lines", "n", 50, "show the last `N` connections and requests")
	f.BoolVarP(&o.Follow, "follow", "f", false, "then show each new one as it happens, until interrupted")
	f.BoolVar(&o.Blocked, "blocked", false, "list the blocks by host and port, with the command that allows each")
	cmd.MarkFlagsMutuallyExclusive("blocked", "follow")
	cmd.MarkFlagsMutuallyExclusive("blocked", "lines")
	return cmd
}
