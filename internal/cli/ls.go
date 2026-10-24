package cli

import (
	"cmp"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/backend/backends"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLsCmd is devvm ls.
func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List the project VMs",
		Long: "List the project VMs on every backend, by name: each one's status, image, .git protection " +
			"(on, off, or clone for a --clone VM), network policy (on, off, or unenforced: not confined by " +
			"devvm), backend and folder. What a VM's state doesn't have (its creation didn't finish) is -.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirs, err := paths.FromEnv()
			var ls []vm.Listed
			if err == nil {
				ls, err = listVMs(cmd, dirs)
			}
			fail(err, msg.LsFailed)
			return writeList(ls)
		},
	}
}

func listVMs(cmd *cobra.Command, dirs paths.Dirs) ([]vm.Listed, error) {
	reg, err := backends.Registry(dirs)
	if err != nil {
		return nil, err
	}
	return vm.List(cmd.Context(), dirs, reg)
}

// writeList prints the VMs as a table, a header first; a value a VM lacks
// is -.
func writeList(ls []vm.Listed) error {
	w := tabwriter.NewWriter(ui.Stdout(), 0, 0, 2, ' ', 0)
	row := func(vals ...string) {
		for i, v := range vals {
			vals[i] = cmp.Or(v, "-")
		}
		_, _ = fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	row("NAME", "STATUS", "IMAGE", "GIT", "NET", "BACKEND", "FOLDER")
	for _, l := range ls {
		row(l.Name, l.Status, l.Image, l.Git, l.Net, l.Backend, l.Folder)
	}
	return w.Flush()
}
