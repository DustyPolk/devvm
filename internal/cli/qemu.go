package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/backend/lima"
	"github.com/devvmhq/devvm/internal/ui"
)

// newQEMUCmd is devvm _qemu, which the Lima backend's QEMU wrapper runs on
// Linux hosts. Hidden: it's not for users.
//
//   - _qemu exec WRAPPER ARGS...: run the real QEMU for WRAPPER in devvm's
//     place, its network restricted.
//   - _qemu connect ADDRESS: join standard input and output to ADDRESS
//     (QEMU's guestfwd runs it for each connection to the proxy).
func newQEMUCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:                "_qemu exec WRAPPER [ARGS...] | _qemu connect ADDRESS",
		Short:              "Run QEMU with devvm's network restrictions",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) >= 2 && args[0] == "exec":
				return lima.RunQEMU(args[1], args[2:])
			case len(args) == 2 && args[0] == "connect":
				return lima.Connect(args[1], cmd.InOrStdin(), u.Stdout())
			}
			return errors.New("usage: devvm " + cmd.Use)
		},
	}
}
