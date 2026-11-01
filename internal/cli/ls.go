package cli

import (
	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/registry"
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
		Long: "List the project VMs on every backend, by name: each one's status, image, .git protection " +
			"(on, off, or clone for a --clone VM), network policy (on, off, or unenforced: not confined by " +
			"devvm), backend and folder. A docker VM made for the docker layer shows its runtime beside its " +
			"backend: docker (sysbox). What a VM's state doesn't have (its creation didn't finish) is -. " +
			"A VM whose backend sizes no disks (docker) and that holds more than VMs get elsewhere is warned about.",
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dirs, err := paths.FromEnv()
			var ls []vm.Listed
			if err == nil {
				ls, err = listVMs(cmd, u, dirs)
			}
			fail(u, err, msg.LsFailed)
			if asJSON {
				err = printJSON(u, lsJSON(ls))
			} else {
				err = writeList(u.Table(), ls)
			}
			if err != nil {
				return err
			}
			for _, l := range ls {
				if l.DiskOver != nil {
					u.Warn(*l.DiskOver)
				}
			}
			return nil
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
	Name    string `json:"name"`
	Status  string `json:"status"`
	Image   string `json:"image"`
	From    string `json:"from,omitempty"`
	Git     string `json:"git"`
	Net     string `json:"net"`
	Backend string `json:"backend"`
	// DockerLayer is the runtime a docker VM runs the docker layer under.
	DockerLayer string `json:"docker_layer,omitempty"`
	Folder      string `json:"folder"`
}

func lsJSON(ls []vm.Listed) lsOutput {
	out := lsOutput{VMs: make([]vmJSON, 0, len(ls))}
	for _, l := range ls {
		out.VMs = append(out.VMs, vmJSON{
			Name: l.Name, Status: l.Status, Image: l.Image, From: l.From, Git: l.Git, Net: l.Net,
			Backend: l.Backend, DockerLayer: l.DockerLayer, Folder: l.Folder,
		})
	}
	return out
}

func listVMs(cmd *cobra.Command, u *ui.UI, dirs paths.Dirs) ([]vm.Listed, error) {
	reg, err := registry.New(dirs, assets.FS, u)
	if err != nil {
		return nil, err
	}
	return vm.List(cmd.Context(), dirs, reg)
}

// writeList writes the VMs as a table, a header first; a value a VM lacks
// is -. A VM's status, .git protection and network policy are colored by
// what they mean.
func writeList(tab *ui.Table, ls []vm.Listed) error {
	tab.Header("NAME", "STATUS", "IMAGE", "GIT", "NET", "BACKEND", "FOLDER")
	for _, l := range ls {
		backend := l.Backend
		if l.DockerLayer != "" {
			backend += " (" + l.DockerLayer + ")"
		}
		image := l.Image
		if l.From != "" {
			image = msg.ImageFrom(l.From, l.Image).Line()
		}
		tab.Row(cell(l.Name), cellAs(l.Status, vmStatuses), cell(image), cellAs(l.Git, gitModes),
			cellAs(l.Net, netModes), cell(backend), cell(l.Folder))
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
	case gitprotect.On, gitprotect.Strict, vm.ModeClone:
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
