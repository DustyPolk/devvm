package vm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/state"
)

// Leftovers are what devvm keeps for VMs their backend no longer has, a
// VM deleted outside devvm (limactl delete, smolvm machine delete, docker
// rm): devvm gc --orphans.
type Leftovers struct{ *Env }

// leftoverRecent is how old a VM's state must be before it can be a
// leftover: a VM being made has its state before its backend has it.
const leftoverRecent = time.Hour

// leftoverAsk bounds what each backend is asked, so one that hangs makes
// its VMs unknown.
const leftoverAsk = 30 * time.Second

// Orphan is a VM whose state devvm keeps but whose backend, asked, no
// longer has it.
type Orphan struct {
	Name, Backend string
	Dir           string    // its project folder
	Changed       time.Time // when its state last changed
	// Leaves are its backend's own resources that Forget deletes with it
	// (backend.LeftoverLister), such as its Docker volumes.
	Leaves []backend.Leftover
}

// LeftoverReport is what Find found.
type LeftoverReport struct {
	Orphans []Orphan
	// Unknown are the backends that couldn't be asked (not installed, not
	// running, too slow, or answering what devvm can't read): their VMs
	// are never orphans.
	Unknown []string
	// Elsewhere are the VMs made where their backend doesn't look in this
	// run (another Lima home, HOME or Docker engine), and Recent those
	// whose state changed in the last hour: never orphans.
	Elsewhere, Recent []string
	// Links are clients' links to VMs that have no state, and Sockets the
	// link sessions' sockets nothing listens on (LeftoverSockets).
	Links   []DeadLink
	Sockets []string
}

// Find finds the VMs whose state names a backend that answers and
// doesn't have them, made where that backend looks in this run, whose
// state is over an hour old. It changes nothing.
func (e Leftovers) Find(ctx context.Context) (LeftoverReport, error) {
	var r LeftoverReport
	names, err := state.VMNames(e.Dirs)
	if err != nil {
		return r, err
	}
	have := map[string]map[string]bool{} // by backend, nil when it can't be asked
	for _, n := range names {
		st, err := state.ReadVM(e.Dirs, n)
		if err != nil {
			return r, err
		}
		bname := st.Get(state.KeyBackend)
		b, ok := e.Registry.Lookup(bname)
		if !ok {
			r.unknown(bname)
			continue
		}
		vms, asked := have[bname]
		if !asked {
			vms = backendVMs(ctx, b)
			have[bname] = vms
		}
		if vms == nil {
			r.unknown(bname)
			continue
		}
		if vms[n] {
			continue
		}
		switch gone, err := gone(ctx, b, n); {
		case err != nil:
			r.unknown(bname)
			continue
		case !gone:
			continue
		}
		if !madeHere(st, b) {
			r.Elsewhere = append(r.Elsewhere, n)
			continue
		}
		fi, err := os.Stat(e.Dirs.VM(n).State())
		if err != nil {
			return r, err
		}
		if time.Since(fi.ModTime()) < leftoverRecent {
			r.Recent = append(r.Recent, n)
			continue
		}
		leaves, err := leftoversOf(ctx, b, n)
		if err != nil {
			return r, err
		}
		r.Orphans = append(r.Orphans, Orphan{Name: n, Backend: bname, Dir: st.Get(state.KeyDir), Changed: fi.ModTime(), Leaves: leaves})
	}
	if r.Links, err = deadLinks(e.Dirs); err != nil {
		return r, err
	}
	r.Sockets, err = LeftoverSockets(ctx, e.Dirs)
	return r, err
}

// ForgetLinks removes every client's links to VMs that have no state, as
// devvm rm of the VM would have, naming the clients that lose them, and
// deletes the link sessions' sockets nothing listens on. Both are found
// again first.
func (e Leftovers) ForgetLinks(ctx context.Context) error {
	dead, err := deadLinks(e.Dirs)
	if err != nil {
		return err
	}
	var targets []string
	for _, l := range dead {
		if !slices.Contains(targets, l.Target) {
			targets = append(targets, l.Target)
		}
	}
	for _, t := range targets {
		if err := linkRuns(e).unlinkTarget(ctx, t); err != nil {
			return err
		}
	}
	socks, err := LeftoverSockets(ctx, e.Dirs)
	if err != nil {
		return err
	}
	for _, s := range socks {
		if err := RemoveLeftoverSocket(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// leftoversOf are the resources of b's own that Forget deletes for vm;
// none for a backend that keeps none (not a backend.LeftoverLister).
func leftoversOf(ctx context.Context, b backend.Backend, vm string) ([]backend.Leftover, error) {
	l, ok := b.(backend.LeftoverLister)
	if !ok {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, leftoverAsk)
	defer cancel()
	return l.Leftovers(ctx, vm)
}

func (r *LeftoverReport) unknown(b string) {
	for _, u := range r.Unknown {
		if u == b {
			return
		}
	}
	r.Unknown = append(r.Unknown, b)
}

// backendVMs is the VMs b has, by name; nil when it can't be asked.
func backendVMs(ctx context.Context, b backend.Backend) map[string]bool {
	ctx, cancel := context.WithTimeout(ctx, leftoverAsk)
	defer cancel()
	if b.Check(ctx) != nil {
		return nil
	}
	ins, err := b.List(ctx)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, in := range ins {
		out[in.Name] = true
	}
	return out
}

// gone asks b for vm itself: true only when b says it has no such VM.
func gone(ctx context.Context, b backend.Backend, vm string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, leftoverAsk)
	defer cancel()
	_, err := b.Inspect(ctx, vm)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, backend.ErrNotFound) {
		return true, nil
	}
	return false, err
}

// madeHere reports whether a VM was made where its backend looks in this
// run: where its state records, or, for a VM that records none, when
// the backend looks in its default place.
func madeHere(st *state.VM, b backend.Backend) bool {
	cur, isDefault := backend.LocationOf(b)
	if at := st.Get(KeyBackendAt); at != "" {
		return at == cur
	}
	return isDefault
}

// Forget deletes what devvm keeps for an orphan, as devvm rm does after
// deleting a VM (teardown), after asking its backend once more that it
// has no such VM, its backend's own leftovers (backend.Forgetter) with
// it.
func (e Leftovers) Forget(ctx context.Context, o Orphan) error {
	st, err := state.ReadVM(e.Dirs, o.Name)
	if err != nil {
		return err
	}
	b, ok := e.Registry.Lookup(st.Get(state.KeyBackend))
	if !st.Exists() || !ok || b.Name() != o.Backend {
		return fmt.Errorf("%s: its state changed", o.Name)
	}
	if isGone, err := gone(ctx, b, o.Name); err != nil {
		return err
	} else if !isGone {
		return fmt.Errorf("%s: its backend has it", o.Name)
	}
	t := &found{f: project.Folder{Name: o.Name, Dir: st.Get(state.KeyDir)}, st: st, b: b}
	return vmFolders(e).teardown(ctx, t, true)
}
