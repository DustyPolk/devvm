package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vm"
)

// newExportCmd is devvm export [FILE].
func newExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export [FILE]",
		Short: "Save this folder's VM's agent history to a .tar.gz",
		Long: "Save this folder's VM's agent history (transcripts and sessions, without the agents' " +
			"logins) to FILE, a .tar.gz, or to a new file in ~/.local/share/devvm/exports. A stopped " +
			"VM is started for it. FILE is replaced only once the export is complete.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(func(env *vm.Env) error {
				file := ""
				if len(args) == 1 {
					abs, err := filepath.Abs(args[0])
					if err != nil {
						return err
					}
					file = abs
				}
				return env.Export(cmd.Context(), file)
			}, msg.ExportFailed)
			return nil
		},
	}
}
