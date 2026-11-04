package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLayerCmd is devvm layer: new NAME and add LAYER....
func newLayerCmd(u *ui.UI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "layer",
		Short: "Make image layers of your own, or add layers to this folder's VM",
		Long: "Make image layers of your own, which are scripts in ~/.config/devvm/layers that an image runs on top of base, or add layers to this folder's VM. " +
			"An image names them as it names devvm's built-in layers, bottom first, such as docker+NAME, and a new VM gets them with --layers.",
		Example: examples(
			"devvm layer new mailpit", "start a layer named mailpit",
			"devvm layer add docker", "make this folder's VM again with the docker layer added",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newLayerAddCmd(u))
	cmd.AddCommand(&cobra.Command{
		Use:   "new NAME",
		Short: "Start a layer of your own",
		Long: "Start a layer of your own, ~/.config/devvm/layers/NAME.sh, a script to edit, and NAME.policy next to it, " +
			"the hosts that VMs with the layer can reach with network policy. " +
			"NAME is lowercase letters, digits and dashes, and not base or a built-in layer's or shared tool's name.",
		Example: examples(
			"devvm layer new mailpit", "start the mailpit layer, then edit its script",
			"devvm build mailpit", "build its image",
			"devvm --layers mailpit", "create a VM with the layer",
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

// newLayerAddCmd is devvm layer add LAYER... [--force] [--no-export]
// [--clean-home].
func newLayerAddCmd(u *ui.UI) *cobra.Command {
	var o vm.RemakeOptions
	cmd := &cobra.Command{
		Use:   "add LAYER...",
		Short: "Make this folder's VM again with more layers, then open a shell in it",
		Long: "Make this folder's VM again from an image with each LAYER added on top, in the order given, then open a shell in it, after asking in a terminal. " +
			"devvm keeps its home folder, the project folder and its settings, and everything else on its disk is lost. " +
			"When its image has an agent, devvm first exports the agents' history, unless you give --no-export.",
		Example: examples(
			"devvm layer add desktop", "add the desktop layer to the VM, after asking",
			"devvm layer add docker nix", "add docker, then nix on top of it",
			"devvm layer add desktop --force", "add it without asking, as a script must",
			"devvm layer add docker --clean-home", "add docker, and start from the image's home folder",
		),
		Args: cobra.MatchAll(minArgs(1), func(_ *cobra.Command, args []string) error {
			dirs, err := paths.FromEnv()
			if err != nil {
				return err
			}
			return vm.CheckLayers(args, (&build.Env{Dirs: dirs}).Layers()[1:])
		}),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := "devvm layer add " + strings.Join(args, " ")
			withEnv(u, func(env *vm.Env) error {
				return vm.Lifecycle{Env: env}.AddLayers(cmd.Context(), command, args, o)
			}, msg.RemakeFailed)
			return nil
		},
		ValidArgsFunction: completeLayerNames,
	}
	addForceFlag(cmd.Flags(), &o.Force)
	addNoExportFlag(cmd.Flags(), &o.NoExport)
	cmd.Flags().BoolVar(&o.CleanHome, "clean-home", false, "give the VM the image's home folder instead of keeping its own")
	return cmd
}
