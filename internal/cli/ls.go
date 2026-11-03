package cli

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ui"
	"github.com/w3cj/devvm/internal/vm"
)

// newLsCmd is devvm ls.
func newLsCmd(u *ui.UI) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List the project VMs",
		Long: "List the project VMs, sorted by name. " +
			"Each row has the VM's status, image, .git protection, network policy, links to your other VMs and folder.",
		Example: examples(
			"devvm ls", "list every project VM",
			"devvm ls --json", "the same list as JSON, for scripts",
		),
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirs, err := paths.FromEnv()
			var ls []vm.Listed
			if err == nil {
				ls, err = listVMs(cmd, u, dirs)
			}
			fail(u, err, msg.LsFailed)
			if asJSON {
				return printJSON(u, lsJSON(ls))
			}
			return writeList(u.Table(), ls)
		},
	}
	jsonFlag(cmd, &asJSON)
	return cmd
}

// lsOutput is devvm ls --json: the project VMs, by name. What a VM's
// state doesn't have is "".
type lsOutput struct {
	VMs []vmJSON `json:"vms"`
}

type vmJSON struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Image  string `json:"image"`
	// Tools is its shared tools: node, claude.
	Tools   []string `json:"tools"`
	From    string   `json:"from,omitempty"`
	Git     string   `json:"git"`
	Net     string   `json:"net"`
	Backend string   `json:"backend"`
	Folder  string   `json:"folder"`
	// Links is how many links the VM has to other VMs' ports.
	Links int `json:"links"`
}

func lsJSON(ls []vm.Listed) lsOutput {
	out := lsOutput{VMs: make([]vmJSON, 0, len(ls))}
	for _, l := range ls {
		out.VMs = append(out.VMs, vmJSON{
			Name: l.Name, Status: l.Status, Image: l.Image, Tools: append([]string{}, l.Tools...), From: l.From, Git: l.Git, Net: l.Net,
			Backend: l.Backend, Folder: l.Folder, Links: l.Links,
		})
	}
	return out
}

func listVMs(cmd *cobra.Command, u *ui.UI, dirs paths.Dirs) ([]vm.Listed, error) {
	reg, err := newRegistry(dirs, assets.FS, u)
	if err != nil {
		return nil, err
	}
	return vm.List(cmd.Context(), dirs, reg)
}

// writeList writes the VMs as a table, a header first; a value a VM lacks
// is -, and so are TOOLS for a VM without shared tools and LINKS for one
// without links. A VM's status, .git protection and network policy are
// colored by what they mean.
func writeList(tab *ui.Table, ls []vm.Listed) error {
	tab.Header("NAME", "STATUS", "IMAGE", "TOOLS", "GIT", "NET", "LINKS", "FOLDER")
	for _, l := range ls {
		image := l.Image
		if l.From != "" {
			image = msg.ImageFrom(l.From, l.Image).Line()
		}
		links := ""
		if l.Links > 0 {
			links = strconv.Itoa(l.Links)
		}
		tab.Row(cell(l.Name), cellAs(l.Status, vmStatuses), cell(image), cell(strings.Join(l.Tools, ",")), cellAs(l.Git, gitModes),
			cellAs(l.Net, netModes), cell(links), cell(l.Folder))
	}
	return tab.Flush()
}

// vmStatuses, gitModes and netModes color devvm ls's STATUS, GIT and NET
// columns: a status other than running or stopped (Broken) is red, and so
// is a network policy that isn't enforced.
func vmStatuses(s string) ui.Role {
	switch backend.Status(s) {
	case backend.StatusRunning:
		return ui.Good
	case backend.StatusStopped:
		return ui.Attention
	case "":
		return ui.Plain
	}
	return ui.Bad
}

func gitModes(s string) ui.Role {
	switch s {
	case gitprotect.On, gitprotect.Strict, gitprotect.Clone:
		return ui.Good
	}
	return ui.Plain
}

func netModes(s string) ui.Role {
	switch s {
	case "on":
		return ui.Good
	case netpolicy.CheckUnenforced:
		return ui.Bad
	}
	return ui.Plain
}
