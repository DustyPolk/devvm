package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
)

// newProxyCmd is devvm _proxy: one VM's network policy proxy, run with
// its config (-config FILE). Hidden: it's not for users. Its flag is the
// proxy's own (one dash or two), parsed by proxy.Main; its event log and
// log lines are data, written to devvm's stdout and stderr. Its built-in
// policies (its config's) come from devvm's own files in the binary, and
// its secrets' values from devvm's store, looked up in this process.
func newProxyCmd(a *app) *cobra.Command {
	u := a.u
	return &cobra.Command{
		Use:                "_proxy [flags]",
		Short:              "Run a VM's network policy proxy",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := a.secrets()
			if err != nil {
				u.Die(msg.ProxyFailed(err.Error()).Because(err))
			}
			err = proxy.Main(args, proxy.IO{
				Stdout: u.Stdout(),
				Stderr: u.Stderr(),
				FS:     assets.FS, // the built-in policies, read from the binary
				Secret: store.Get,
			})
			if err != nil {
				u.Die(msg.ProxyFailed(err.Error()).Because(err))
			}
			return nil
		},
	}
}
