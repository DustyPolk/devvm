package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/ui"
)

// newProxyCmd is devvm _proxy: one VM's network policy proxy, and the
// one-shot modes devvm runs it in (-init-ca, -login, -redact). Hidden: it's
// not for users. Its flags are the proxy's own (one dash or two), parsed by
// proxy.Main; its event log and log lines are data, written to devvm's
// stdout and stderr.
func newProxyCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "_proxy [flags]",
		Short:              "Run a VM's network policy proxy",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := proxy.Main(args, proxy.IO{
				Stdin:  cmd.InOrStdin(),
				Stdout: ui.Stdout(),
				Stderr: ui.Stderr(),
				Kept:   func(agent string, names []string) { secrets.LoginKept(ui.Warn, agent, names) },
			})
			if err != nil {
				ui.Die(msg.ProxyFailed(err.Error()).Because(err))
			}
			return nil
		},
	}
}
