package cli

import (
	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/clipboard"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/vm"
)

// newClipboardCmd is devvm clipboard [images|off].
func newClipboardCmd(a *app) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "clipboard [images|off]",
		Short: "Show or change whether this folder's VM reads your clipboard's images",
		Long: "Show whether this folder's VM reads the image on your clipboard when you paste into a coding agent in it, as images or off. " +
			"devvm clipboard images or off changes it at once, on a running VM too. " +
			"With images on, anything in the VM can read the image on your clipboard while the VM runs, but never its text.",
		Example: examples(
			"devvm clipboard", "show whether the VM reads your clipboard's images",
			"devvm clipboard images", "let agents in the VM paste the image on your clipboard",
			"devvm clipboard off", "stop it",
		),
		Args:      showsOnly(maxArgs(1)),
		ValidArgs: clipboard.Modes,
		RunE: func(cmd *cobra.Command, args []string) error {
			var mode string
			err := a.env(func(env *vm.Env) error {
				s := vm.Settings{Env: env}
				if len(args) == 1 {
					return s.SetClipboard(cmd.Context(), args[0])
				}
				var err error
				mode, err = s.Clipboard(cmd.Context())
				return err
			}, msg.ClipboardFailed)
			if err != nil || len(args) == 1 {
				return err
			}
			return printValue(a.u, asJSON, "clipboard", mode)
		},
	}
	jsonFlag(cmd, &asJSON)
	return addFolderFlags(cmd, a)
}
