package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// newLayerCmd is devvm layer new NAME.
func newLayerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layer",
		Short: "Your own image layers",
		Long: "Your own image layers: scripts in ~/.config/devvm/layers that an image runs on top of base, " +
			"named in images like the shared tools (node+NAME).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "new NAME",
		Short: "Start a layer of your own",
		Long: "Start a layer: ~/.config/devvm/layers/NAME.sh, a script to edit, and NAME.policy next to it, " +
			"the hosts VMs with the layer can reach with network policy (kept if it exists). NAME is " +
			"lowercase letters, digits and dashes, and not base or a shared tool's. Then devvm build NAME " +
			"builds its image, and devvm node+NAME makes a VM with it.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			var script, policy string
			if err == nil {
				script, policy, err = build.NewLayer(dirs, args[0])
			}
			fail(err, msg.LayerNewFailed)
			ui.Info(msg.LayerCreated(args[0], policy, script))
			return nil
		},
	})
	return cmd
}
