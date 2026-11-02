package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLinkCmd is devvm link: this folder's VM's links to other VMs, listed,
// with add and rm, and publish and unpublish for the ports it offers to
// theirs.
func newLinkCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "link",
		Short: "List this folder's VM's links to your other VMs",
		Long: "List this folder's VM's links to services in your other VMs, each with its name, port, target VM and folder, " +
			"the target's status, and whether the link applies. " +
			"Programs in the VM reach a link as NAME:PORT and NAME.devvm.internal:PORT.",
		Example: examples(
			"devvm link", "list this folder's VM's links",
			"devvm link add db 5432", "reach port 5432 of the VM for the folder db as db:5432",
			"devvm link publish 5432", "offer this VM's port 5432 to links from your other VMs",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var ls []vm.LinkListed
			withEnv(u, func(env *vm.Env) error {
				var err error
				ls, err = vm.Links{Env: env}.List(cmd.Context())
				return err
			}, msg.LinkFailed)
			if asJSON {
				return printJSON(u, linkJSON(ls))
			}
			return writeLinks(u.Table(), ls)
		},
	}
	jsonFlag(cmd, &asJSON)
	cmd.AddCommand(newLinkAddCmd(u), newLinkRmCmd(u), newLinkPublishCmd(u), newLinkUnpublishCmd(u))
	return cmd
}

// linkOutput is devvm link --json: the VM's links, each with its target's
// folder and status ("" when unknown) and whether it applies, with why
// not (gone, replaced, unpublished, policy or ungrouped).
type linkOutput struct {
	Links []linkEntryJSON `json:"links"`
}

type linkEntryJSON struct {
	Name         string `json:"name"`
	Port         int    `json:"port"`
	Target       string `json:"target"`
	TargetFolder string `json:"target_folder"`
	TargetStatus string `json:"target_status"`
	Applies      bool   `json:"applies"`
	Reason       string `json:"reason"`
}

func linkJSON(ls []vm.LinkListed) linkOutput {
	out := linkOutput{Links: make([]linkEntryJSON, 0, len(ls))}
	for _, l := range ls {
		out.Links = append(out.Links, linkEntryJSON{
			Name: l.Name, Port: l.Port, Target: l.Target, TargetFolder: l.Folder, TargetStatus: l.Status,
			Applies: l.Reason == "", Reason: l.Reason,
		})
	}
	return out
}

// writeLinks writes the links as a table, a header first: APPLIES is yes,
// or why not.
func writeLinks(tab *ui.Table, ls []vm.LinkListed) error {
	tab.Header("NAME", "PORT", "TARGET", "FOLDER", "STATUS", "APPLIES")
	for _, l := range ls {
		applies := "yes"
		if l.Reason != "" {
			applies = l.Reason
		}
		tab.Row(cell(l.Name), cell(strconv.Itoa(l.Port)), cell(l.Target), cell(l.Folder), cell(l.Status), cell(applies))
	}
	return tab.Flush()
}

// linkPortArg checks a link's PORT, the command's second argument.
func linkPortArg(_ *cobra.Command, args []string) error {
	_, err := vm.ParseLinkPort(args[1])
	return err
}

func newLinkAddCmd(u *ui.UI) *cobra.Command {
	var as string
	cmd := &cobra.Command{
		Use:   "add TARGET PORT",
		Short: "Link this folder's VM to a port of another VM",
		Long: "Link this folder's VM to PORT of the VM TARGET names, a folder's name or path or a VM's name, " +
			"once that VM publishes the port with devvm link publish. " +
			"Programs in this VM then reach it as NAME:PORT and NAME.devvm.internal:PORT, where NAME is the target's folder name or --as NAME.",
		Example: examples(
			"devvm link add db 5432", "reach port 5432 of the VM for the folder db as db:5432",
			"devvm link add db 5432 --as pg", "reach it as pg:5432 instead",
			"devvm link add ../api 8080", "name the target by its folder's path",
		),
		Args: cobra.MatchAll(exactArgs(2), linkPortArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := vm.ParseLinkPort(args[1])
			withEnv(u, func(env *vm.Env) error {
				return vm.Links{Env: env}.Add(cmd.Context(), vm.LinkAdd{Target: args[0], Port: port, As: as})
			}, msg.LinkFailed)
			return nil
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "the `NAME` the VM reaches the link by (default: the target's folder name)")
	return cmd
}

func newLinkRmCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "rm TARGET PORT",
		Short: "Remove one of this folder's VM's links",
		Long: "Remove this folder's VM's link to PORT of TARGET, " +
			"where TARGET is the link's name or what devvm link add took.",
		Example: examples(
			"devvm link rm db 5432", "stop reaching db:5432",
			"devvm link rm pg 5432", "remove a link by the name given with --as",
		),
		Args: cobra.MatchAll(exactArgs(2), linkPortArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := vm.ParseLinkPort(args[1])
			withEnv(u, func(env *vm.Env) error { return vm.Links{Env: env}.Rm(cmd.Context(), args[0], port) }, msg.LinkFailed)
			return nil
		},
	}
}

// publishArg checks the ports devvm link publish and unpublish take.
func publishArg(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	_, err := vm.ParsePublish(args[0])
	return err
}

func newLinkPublishCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "publish [PORTS]",
		Short: "Show or add the ports this folder's VM offers to links",
		Long: "Show the ports this folder's VM publishes, or offer PORTS, ports and ranges like 5432,8000-8010, " +
			"to links from any of your other VMs. " +
			"Each link is still made from the other VM's folder, with devvm link add.",
		Example: examples(
			"devvm link publish", "show what the VM publishes",
			"devvm link publish 5432", "offer port 5432 to links",
			"devvm link publish 8000-8010", "offer a range",
		),
		Args: showsOnly(cobra.MatchAll(maxArgs(1), publishArg)),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ports string
			withEnv(u, func(env *vm.Env) error {
				l := vm.Links{Env: env}
				if len(args) == 1 {
					return l.Publish(cmd.Context(), args[0], true)
				}
				var err error
				ports, err = l.Published(cmd.Context())
				return err
			}, msg.LinkFailed)
			if len(args) == 1 {
				return nil
			}
			return printValue(u, asJSON, "ports", ports)
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

func newLinkUnpublishCmd(u *ui.UI) *cobra.Command {
	return &cobra.Command{
		Use:   "unpublish PORTS",
		Short: "Stop offering ports of this folder's VM to links",
		Long: "Stop offering PORTS, each as devvm link publish lists it, to links from your other VMs. " +
			"The server in the VM keeps running.",
		Example: examples(
			"devvm link unpublish 5432", "stop offering port 5432",
		),
		Args: cobra.MatchAll(exactArgs(1), publishArg),
		RunE: func(cmd *cobra.Command, args []string) error {
			withEnv(u, func(env *vm.Env) error { return vm.Links{Env: env}.Publish(cmd.Context(), args[0], false) }, msg.LinkFailed)
			return nil
		},
	}
}
