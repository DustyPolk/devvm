package vm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmssh"
)

// Why a link's connection is refused, besides CheckLink's reasons, as its
// event has it (vmssh.EventLinkRefused).
const (
	LinkRemoved = "removed" // the client no longer has the link
	LinkNoSSH   = "ssh"     // the target's ssh can't be reached as devvm pins it
	linkUnread  = "unread"  // the states can't be read
	// LinkUnenforced: the client has network policy, and the target's,
	// though on, isn't enforced (netpolicy.Listed): it runs outside its
	// proxy, until its next start, or its self-check failed.
	LinkUnenforced = "unenforced"
)

// How often a link session asks whether the client still runs as it
// did, and reads its links again.
const (
	linkAliveEvery = 5 * time.Second
	linkSyncEvery  = time.Second
)

// Session carries client vm's links while it runs, as devvm _links does,
// detached, its lines going to log: the client reached over its own ssh,
// as devvm pins it, and each connection checked against the states as
// they are then (linkConn). It ends once the client stops, or has no
// links.
func (e Links) Session(ctx context.Context, vm string, log io.Writer) error {
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		return err
	}
	if !st.Exists() {
		return fmt.Errorf("%s: no such VM", vm)
	}
	c, err := e.Registry.For(st)
	if err != nil {
		return err
	}
	b := c.Backend
	if err := e.Registry.Require(b, vm, linkFeatures()...); err != nil {
		return err
	}
	if in, err := b.Inspect(ctx, vm); err != nil || in.Status != backend.StatusRunning {
		return msg.VMNotRunning(vm)
	}
	start, err := b.StartID(ctx, vm)
	if err != nil {
		return err
	}
	if start == "" {
		return msg.VMNotRunning(vm)
	}
	ssh, err := e.linkSSH(ctx, b, vm)
	if err != nil {
		return err
	}
	events := linkEvents{path: e.Dirs.VM(vm).EventLog(), vm: vm, mu: &sync.Mutex{}}
	s := &vmssh.Links{
		VM: vm, SSH: ssh, Dir: e.Dirs.Run(), Log: log, List: e.Dirs.VM(vm).LinkList(),
		Every: linkAliveEvery, Sync: linkSyncEvery, Tick: 2 * time.Second,
		Alive: func(ctx context.Context) bool {
			id, err := b.StartID(ctx, vm)
			return err == nil && id == start
		},
		Want:  func() ([]vmssh.Link, error) { return e.sessionLinks(vm) },
		Check: e.linkCheck(vm),
		Event: events.add,
		Failed: func(l vmssh.Link, err error) {
			e.Warn(Link{Name: l.Name, Port: l.Port}.PortTaken(vm).Because(err))
		},
	}
	return s.Run(ctx)
}

// sessionLinks returns a client's links as its session carries them:
// every one its state has, whether it applies now or not, as each
// connection is checked again, with the names its proxy takes it by. A
// client that's gone has none.
func (e Links) sessionLinks(vm string) ([]vmssh.Link, error) {
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil || !st.Exists() {
		return nil, err
	}
	links, err := ReadLinks(st)
	if err != nil {
		return nil, err
	}
	out := make([]vmssh.Link, len(links))
	for i, l := range links {
		out[i] = vmssh.Link{Name: l.Name, Target: l.Target, Addr: l.Addr, Port: l.Port, Hosts: []string{l.Name, l.Name + "." + proxy.LinkDomain}}
	}
	return out, nil
}

// linkCheck is how client's link session checks each connection: its
// targets' runs shared among the connections for linkRunFor.
func (e Links) linkCheck(client string) func(context.Context, vmssh.Link) (vmssh.LinkTarget, string, string) {
	runs := &targetRuns{For: linkRunFor, Now: time.Now}
	return func(ctx context.Context, l vmssh.Link) (vmssh.LinkTarget, string, string) {
		return e.linkConn(ctx, client, l, runs)
	}
}

// linkRunFor is how long a link session reuses what a target's backend
// said of its run, so a burst of connections asks it once, and a target
// that stops is noticed this long after at most.
const linkRunFor = time.Second

// targetRun is what a target's backend says of its run, as a link's
// connections need it: its start ("" when it doesn't run), and how
// devvm's ssh reaches it, or why it can't (linkSSH).
type targetRun struct {
	start  string
	ssh    vmssh.SSH
	sshErr error
}

// targetRuns shares targets' runs among a link session's connections: one
// lookup in flight for each target, its answer reused for For after it
// ends (none when For is 0). A connection whose context ends while it
// waits, or while its own lookup runs, finds the target stopped; a lookup
// cut short that way isn't kept, so the connections waiting on it look
// again.
type targetRuns struct {
	For time.Duration
	Now func() time.Time

	mu sync.Mutex
	by map[string]*runLookup // by target and its ID
}

// runLookup is one lookup of a target's run: done once run is set, at, or
// once it was cut short (cut), its run unknown.
type runLookup struct {
	done chan struct{}
	run  targetRun
	at   time.Time
	cut  bool
}

// get returns the run of the target keyed key, from the lookup in flight
// for it or one that ended less than For ago, else from look.
func (r *targetRuns) get(ctx context.Context, key string, look func(context.Context) targetRun) targetRun {
	for {
		lk, mine := r.lookup(key)
		if mine {
			return r.run(ctx, key, lk, look)
		}
		select {
		case <-lk.done:
			if !lk.cut {
				return lk.run
			}
		case <-ctx.Done():
			return targetRun{}
		}
	}
}

// lookup returns the lookup in flight for key or one that ended less than
// For ago, or else a new one the caller runs (mine).
func (r *targetRuns) lookup(key string) (lk *runLookup, mine bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if lk := r.by[key]; lk != nil {
		select {
		case <-lk.done:
			if r.Now().Sub(lk.at) < r.For {
				return lk, false
			}
		default:
			return lk, false
		}
	}
	lk = &runLookup{done: make(chan struct{})}
	if r.by == nil {
		r.by = map[string]*runLookup{}
	}
	r.by[key] = lk
	return lk, true
}

// run runs lk, key's lookup, with look. One whose context ended is cut
// and dropped before it's done, so no connection reuses it.
func (r *targetRuns) run(ctx context.Context, key string, lk *runLookup, look func(context.Context) targetRun) targetRun {
	run := look(ctx)
	if ctx.Err() != nil {
		r.mu.Lock()
		if r.by[key] == lk {
			delete(r.by, key)
		}
		r.mu.Unlock()
		lk.cut = true
		close(lk.done)
		return targetRun{}
	}
	lk.run, lk.at = run, r.Now()
	close(lk.done)
	return run
}

// targetRun asks a target's backend whether it runs, its start, and how
// devvm's ssh reaches it.
func (e Links) targetRun(ctx context.Context, b backend.Backend, target string) targetRun {
	if in, err := b.Inspect(ctx, target); err != nil || in.Status != backend.StatusRunning {
		return targetRun{}
	}
	start, err := b.StartID(ctx, target)
	if err != nil || start == "" {
		return targetRun{}
	}
	ssh, err := e.linkSSH(ctx, b, target)
	return targetRun{start: start, ssh: ssh, sshErr: err}
}

// linkConn checks a connection to one of client's links, from the states
// as they are now: the client still has the link, it applies (CheckLink:
// the target's ID, its published port, the policy rule), the target's
// backend has links, and the target runs, as its backend said within
// runs' time, its network policy enforced when the client has it
// (policyEnforced), and is reached as devvm pins it. It returns how to
// reach the target, or the event and why not. It never starts the
// target.
func (e Links) linkConn(ctx context.Context, client string, l vmssh.Link, runs *targetRuns) (vmssh.LinkTarget, string, string) {
	refused := func(why string, err error) (vmssh.LinkTarget, string, string) {
		if err != nil {
			why += ": " + err.Error()
		}
		return vmssh.LinkTarget{}, vmssh.EventLinkRefused, why
	}
	cst, err := state.ReadVM(e.Dirs, client)
	if err != nil {
		return refused(linkUnread, err)
	}
	links, err := ReadLinks(cst)
	if err != nil {
		return refused(linkUnread, err)
	}
	i := slices.IndexFunc(links, func(k Link) bool { return k.Addr == l.Addr && k.Port == l.Port })
	if i < 0 {
		return refused(LinkRemoved, nil)
	}
	why, tst, err := CheckLink(e.Dirs, cst, links[i])
	switch {
	case err != nil:
		return refused(linkUnread, err)
	case why != "":
		return refused(why, nil)
	}
	c, err := e.Registry.For(tst)
	if err != nil {
		return refused(linkUnread, err)
	}
	target := tst.Name()
	if err := e.Registry.Require(c.Backend, target, linkFeatures()...); err != nil {
		return refused(LinkNoSSH, err)
	}
	run := runs.get(ctx, target+" "+tst.ID, func(ctx context.Context) targetRun {
		return e.targetRun(ctx, c.Backend, target)
	})
	if run.start == "" {
		return vmssh.LinkTarget{}, vmssh.EventLinkStopped, ""
	}
	if why, err := policyEnforced(ctx, c.Backend, cst, tst); err != nil || why != "" {
		return refused(cmp.Or(why, linkUnread), err)
	}
	if run.sshErr != nil {
		return refused(LinkNoSSH, run.sshErr)
	}
	return vmssh.LinkTarget{SSH: run.ssh, Start: run.start}, "", ""
}

// policyEnforced checks the policy rule against a running target as it
// runs, not only as its state has it: for a client with network policy,
// the target's must be enforced, as devvm ls shows it. A target given
// network policy while it ran still runs outside its proxy until its next
// start, and one whose self-check failed may reach the internet directly,
// so either would carry the client out (LinkUnenforced).
func policyEnforced(ctx context.Context, b backend.Backend, client, target *state.VM) (string, error) {
	if !netpolicy.On(client) {
		return "", nil
	}
	listed, err := netpolicy.Listed(ctx, b, target, true)
	switch {
	case err != nil:
		return "", err
	case listed != "on":
		return LinkUnenforced, nil
	}
	return "", nil
}

// policyUnenforced reports whether a running target's network policy
// isn't in force for a link from client (policyEnforced), so the link's
// connections are refused until the target starts again. It only decides
// a warning: a target whose policy can't be read isn't warned of, as each
// connection checks it again.
func policyUnenforced(ctx context.Context, b backend.Backend, client, target *state.VM) bool {
	why, err := policyEnforced(ctx, b, client, target)
	return err == nil && why == LinkUnenforced
}

// warnUnenforced warns when client's links to target, found as devvm
// link add and publish check them, are refused for now because the target
// runs with its network policy not in force (policyUnenforced). A stopped
// target isn't warned of, as its policy applies when it next starts.
func (e Links) warnUnenforced(ctx context.Context, client, target *state.VM) {
	if !netpolicy.On(client) {
		return
	}
	c, err := e.Registry.For(target)
	if err != nil {
		return
	}
	if in, err := c.Backend.Inspect(ctx, target.Name()); err != nil || in.Status != backend.StatusRunning {
		return
	}
	if policyUnenforced(ctx, c.Backend, client, target) {
		e.Warn(msg.LinkTargetUnenforced(target.Name(), client.Name()))
	}
}

// linkSSH is how devvm's own ssh reaches a running VM for links: its
// backend's ssh (reviewed), with the VM's own key and the host key devvm
// pins for it, on a host loopback port.
func (e Links) linkSSH(ctx context.Context, b backend.Backend, vm string) (vmssh.SSH, error) {
	if !e.Registry.Allowed(b, backend.FeatureSSH) {
		return vmssh.SSH{}, fmt.Errorf("%s: the %s backend's ssh isn't usable", vm, b.Name())
	}
	t, err := b.SSH(ctx, vm)
	if err != nil {
		return vmssh.SSH{}, err
	}
	key := ""
	for _, o := range t.Options {
		if strings.EqualFold(o.Key, "IdentityFile") {
			key = strings.Trim(o.Value, `"`)
			break
		}
	}
	switch {
	case t.User == "" || key == "":
		return vmssh.SSH{}, fmt.Errorf("%s: no user or key for its ssh", vm)
	case t.Host != "127.0.0.1" || t.Port <= 0:
		return vmssh.SSH{}, fmt.Errorf("%s: its ssh isn't on a host loopback port", vm)
	}
	pinned, err := hostKeyPinned(e.Dirs.KnownHosts(), vm)
	if err != nil {
		return vmssh.SSH{}, err
	}
	if !pinned {
		return vmssh.SSH{}, fmt.Errorf("%s: no host key pinned", vm)
	}
	return vmssh.SSH{VM: vm, User: t.User, Key: key, KnownHosts: e.Dirs.KnownHosts(), Port: t.Port}, nil
}

// hostKeyPinned reports whether devvm's known hosts file has a host key
// for vm, which devvm's ssh then checks the VM's against (HostKeyAlias).
func hostKeyPinned(path, vm string) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // devvm's own file
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for l := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(l, vm+" ") {
			return true, nil
		}
	}
	return false, nil
}

// linkEvents writes a link session's events to its client's event log,
// one at a time.
type linkEvents struct {
	path, vm string
	mu       *sync.Mutex
}

// add writes one event: the link's name as its host, and its target.
func (le linkEvents) add(event string, l vmssh.Link, why string) {
	le.mu.Lock()
	defer le.mu.Unlock()
	_ = netpolicy.AppendEvent(le.path, le.vm, netpolicy.Event{Event: event, Host: l.Name, Port: l.Port, Target: l.Target, Reason: why})
}
