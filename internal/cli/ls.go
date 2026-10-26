package cli

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend/registry"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLsCmd is devvm ls.
func newLsCmd(u *ui.UI) *cobra.Command {
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
				ls, err = listVMs(cmd, u, dirs)
			}
			fail(u, err, msg.LsFailed)
			return writeList(u.Stdout(), ls)
		},
	}
}

func listVMs(cmd *cobra.Command, u *ui.UI, dirs paths.Dirs) ([]vm.Listed, error) {
	reg, err := registry.New(dirs, assets.FS, u)
	if err != nil {
		return nil, err
	}
	return vm.List(cmd.Context(), dirs, reg)
}

// writeList writes the VMs to out as a table, a header first; a value a
// VM lacks is -.
func writeList(out io.Writer, ls []vm.Listed) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
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
