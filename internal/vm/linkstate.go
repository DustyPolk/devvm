package vm

import (
	"context"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
)

// clientStates calls each with the state and links of every project VM
// but skip, by name.
func clientStates(d paths.Dirs, skip string, each func(st *state.VM, links []Link) error) error {
	names, err := state.VMNames(d)
	if err != nil {
		return err
	}
	slices.Sort(names)
	for _, n := range names {
		if n == skip || !project.IsVM(n) {
			continue
		}
		st, err := state.ReadVM(d, n)
		if err != nil {
			return err
		}
		links, err := ReadLinks(st)
		if err != nil {
			return err
		}
		if err := each(st, links); err != nil {
			return err
		}
	}
	return nil
}

// linkClients returns the project VMs, by name, with a link to target
// (its VM name and ID) that picks.
func linkClients(d paths.Dirs, target, id string, picks func(client *state.VM, l Link) bool) ([]string, error) {
	var out []string
	err := clientStates(d, target, func(st *state.VM, links []Link) error {
		if slices.ContainsFunc(links, func(l Link) bool { return l.Target == target && l.TargetID == id && picks(st, l) }) {
			out = append(out, st.Name())
		}
		return nil
	})
	return out, err
}

// policyLinked refuses a change of a VM's network policy that would break
// links' policy rule: turning it off (on false) for a target that clients
// with network policy link to, or on for a client that links to a target
// without it. Links to a target that's gone or was made again don't
// count.
func policyLinked(d paths.Dirs, st *state.VM, on bool) error {
	vm := st.Name()
	if !on {
		clients, err := linkClients(d, vm, st.Get(state.KeyID), func(c *state.VM, _ Link) bool { return netpolicy.On(c) })
		if err != nil || len(clients) == 0 {
			return err
		}
		return msg.NetworkOffLinked(vm, strings.Join(clients, ", "))
	}
	links, err := ReadLinks(st)
	if err != nil {
		return err
	}
	var targets []string
	for _, l := range links {
		tst, err := state.ReadVM(d, l.Target)
		if err != nil {
			return err
		}
		if tst.Exists() && tst.Get(state.KeyID) == l.TargetID && !netpolicy.On(tst) && !slices.Contains(targets, l.Target) {
			targets = append(targets, l.Target)
		}
	}
	if len(targets) == 0 {
		return nil
	}
	return msg.NetworkOnLinked(strings.Join(targets, ", "), vm)
}

// unlinkTarget removes every link to target, a VM devvm deletes or
// forgets, from every client's state, names the clients that lost one,
// and gives each running one its links as they are now (apply). One that
// can't be given them is warned of, and gets them at its next start.
func (e linkRuns) unlinkTarget(ctx context.Context, target string) error {
	var lost []*state.VM
	err := clientStates(e.Dirs, target, func(st *state.VM, links []Link) error {
		kept := slices.DeleteFunc(slices.Clone(links), func(l Link) bool { return l.Target == target })
		if len(kept) == len(links) {
			return nil
		}
		lost = append(lost, st)
		return writeLinks(st, kept)
	})
	if err != nil || len(lost) == 0 {
		return err
	}
	names := make([]string, len(lost))
	for i, st := range lost {
		names[i] = st.Name()
	}
	e.Info(msg.LinksTargetGone(len(lost), target, strings.Join(names, ", ")))
	for _, st := range lost {
		c, err := e.Registry.For(e.Dirs, st, "")
		if err == nil {
			_, err = e.apply(ctx, c.Backend, st, nil, 0)
		}
		if err != nil {
			e.Warn(msg.LinksUpdateFailed(st.Name()).Because(err))
		}
	}
	return nil
}

// DeadLink is a client's link to a VM that has no state any more, which
// devvm gc --orphans removes.
type DeadLink struct {
	Client, Name, Target string
	Port                 int
}

// deadLinks returns every client's links to VMs that have no state.
func deadLinks(d paths.Dirs) ([]DeadLink, error) {
	var out []DeadLink
	err := clientStates(d, "", func(st *state.VM, links []Link) error {
		for _, l := range links {
			tst, err := state.ReadVM(d, l.Target)
			if err != nil {
				return err
			}
			if !tst.Exists() {
				out = append(out, DeadLink{Client: st.Name(), Name: l.Name, Target: l.Target, Port: l.Port})
			}
		}
		return nil
	})
	return out, err
}
