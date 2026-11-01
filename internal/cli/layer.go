package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
)

// newLayerCmd is devvm layer new NAME.
func newLayerCmd(u *ui.UI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layer",
		Short: "Make image layers of your own",
		Long: "Make image layers of your own, which are scripts in ~/.config/devvm/layers that an image runs on top of base. " +
			"An image names them as it names the shared tools and devvm's built-in layers, such as node+docker+NAME.",
		Example: examples(
			"devvm layer new mailpit", "start a layer named mailpit",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "new NAME",
		Short: "Start a layer of your own",
		Long: "Start a layer of your own, ~/.config/devvm/layers/NAME.sh, a script to edit, and NAME.policy next to it, " +
			"the hosts that VMs with the layer can reach with network policy. " +
			"NAME is lowercase letters, digits and dashes, and not base or a built-in layer's or shared tool's name.",
		Example: examples(
			"devvm layer new mailpit", "start the mailpit layer, then edit its script",
			"devvm build mailpit", "build its image",
			"devvm node+mailpit", "create a VM with node and the layer",
		),
		Args: cobra.MatchAll(exactArgs(1), func(_ *cobra.Command, args []string) error {
			return build.CheckLayerName(args[0])
		}),
		RunE: func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			var script, policy string
			if err == nil {
				script, policy, err = build.NewLayer(dirs, args[0])
			}
			fail(u, err, msg.LayerNewFailed)
			u.Info(msg.LayerCreated(args[0], policy, script))
			return nil
		},
	})
	return cmd
}
