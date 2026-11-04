package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/ui"
)

// newProxyCmd is devvm _proxy: one VM's network policy proxy, run with
// its config (-config FILE). Hidden: it's not for users. Its flag is the
// proxy's own (one dash or two), parsed by proxy.Main; its event log and
// log lines are data, written to devvm's stdout and stderr. Its built-in
// policies (its config's) come from devvm's own files in the binary, and
// its secrets' values from devvm's store, looked up in this process.
func newProxyCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:                "_proxy [flags]",
		Short:              "Run a VM's network policy proxy",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			if err != nil {
				u.Die(msg.ProxyFailed(err.Error()).Because(err))
			}
			err = proxy.Main(args, proxy.IO{
				Stdout: u.Stdout(),
				Stderr: u.Stderr(),
				FS:     assets.FS, // the built-in policies, read from the binary
				Secret: secrets.NewStore(dirs, u.Warn).Get,
			})
			if err != nil {
				u.Die(msg.ProxyFailed(err.Error()).Because(err))
			}
			return nil
		},
	}
}
