package vm

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
)

// Listed is a project VM as devvm ls shows it. A field is "" when the VM's
// state doesn't record it (its creation didn't finish).
type Listed struct {
	Name    string
	Status  string // as its backend reports it
	Image   string // its image: node+claude, base
	From    string // the container image its base started from (--from), if any
	Git     string // its .git protection: on, off, or clone for a --clone VM
	Net     string // its network policy: on, off or unenforced (netpolicy.Listed)
	Backend string
	// DockerLayer is the runtime it runs the docker layer under, where its
	// backend makes VMs for it (KeyDockerLayer).
	DockerLayer string
	Folder      string
	// DiskOver warns that its disk holds more than VMs get elsewhere, on
	// a backend that sizes no disks (backend.DiskUser); nil otherwise.
	DiskOver *msg.M
}

// List lists the project VMs on every backend, sorted by name. A backend
// that can't list fails it only when a VM's state names that backend;
// otherwise it has none of devvm's VMs (not installed, say).
func List(ctx context.Context, d paths.Dirs, reg *backend.Registry) ([]Listed, error) {
	var out []Listed
	for _, b := range reg.All() {
		ins, err := b.List(ctx)
		if err != nil {
			if used, uerr := state.Backends(d); uerr != nil {
				return nil, uerr
			} else if !used[b.Name()] {
				continue
			}
			if cerr := b.Check(ctx); cerr != nil {
				return nil, cerr
			}
			return nil, err
		}
		for _, in := range ins {
			if !project.IsVM(in.Name) {
				continue
			}
			l, err := listed(ctx, d, b, in)
			if err != nil {
				return nil, err
			}
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(a, b Listed) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Backend, b.Backend))
	})
	return out, nil
}

// listed is one VM's row, from its backend and its state.
func listed(ctx context.Context, d paths.Dirs, b backend.Backend, in backend.Instance) (Listed, error) {
	st, err := state.ReadVM(d, in.Name)
	if err != nil {
		return Listed{}, err
	}
	l := Listed{
		Name: in.Name, Status: string(in.Status), Image: st.Get(state.KeyImage),
		Git: st.Get(gitprotect.KeyGitProtect), Backend: b.Name(), DockerLayer: st.Get(KeyDockerLayer),
		Folder: st.Get(state.KeyDir),
	}
	if l.Image != "" {
		l.From = st.Get(state.KeyFrom)
	}
	if isClone(st) {
		l.Git = ModeClone
	}
	if du, ok := b.(backend.DiskUser); ok {
		// One it can't measure goes unsaid: ls lists it all the same.
		if used, limit, err := du.DiskUsed(ctx, in.Name); err == nil && limit > 0 && used > limit {
			m := msg.DiskOver(b.Name(), gibText(limit), gibText(used), in.Name)
			l.DiskOver = &m
		}
	}
	l.Net, err = netpolicy.Listed(ctx, b, st, in.Status == backend.StatusRunning)
	return l, err
}

// ToolsInUse counts the VMs that list each shared tool image in their
// state.
func ToolsInUse(d paths.Dirs) (map[string]int, error) {
	vms, err := state.VMNames(d)
	if err != nil {
		return nil, err
	}
	n := map[string]int{}
	for _, vm := range vms {
		st, err := state.ReadVM(d, vm)
		if err != nil {
			return nil, err
		}
		for _, f := range strings.Fields(st.Get(KeyTools)) {
			n[f]++
		}
	}
	return n, nil
}
