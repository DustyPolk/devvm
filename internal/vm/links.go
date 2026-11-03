package vm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmname"
)

// Keys of a VM's state for links between VMs.
const (
	// KeyPublish is the ports and ranges the VM offers to links from the
	// user's other VMs (ParsePorts, never all); unset for none.
	KeyPublish = "publish"
	// KeyLinks is the VM's links, as a client: formatLinks's entries.
	KeyLinks = "links"
)

// Link is one of a client VM's links: the client reaches the target's
// Port at Name (and Name under proxy.LinkDomain), on Addr, the address it
// has for that target (proxy.IsLinkAddr).
type Link struct {
	Name     string     // one DNS label (CheckLinkName)
	Port     int        // the target's port, which the client listens on too
	Target   string     // the target VM's name
	TargetID string     // the target's state.KeyID when the link was made
	Addr     netip.Addr // in 127.77.0.0/16, one per target in this client
	Group    bool       // the client's group made it (groupLinks), not devvm link add
}

// linkGroupMark is the sixth field of a link's entry that its group made.
const linkGroupMark = "group"

// linkName is a link's name: one DNS label, in lowercase.
var linkName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// CheckLinkName refuses a name that isn't one DNS label of lowercase
// letters, digits and dashes, or is digits alone, which resolvers can
// read as an address.
func CheckLinkName(name string) error {
	if !linkName.MatchString(name) || strings.Trim(name, "0123456789") == "" {
		return msg.LinkNameInvalid(strconv.Quote(name))
	}
	return nil
}

// ParseLinkPort checks a link's PORT: one port, 1 to 65535.
func ParseLinkPort(v string) (int, error) {
	p, err := strconv.Atoi(v)
	if err != nil || !backend.ValidPort(p) || strconv.Itoa(p) != v {
		return 0, msg.LinkPortInvalid(strconv.Quote(v))
	}
	return p, nil
}

// ParsePublish checks the ports devvm link publish and unpublish take:
// ports and ranges, never all or none. It returns them as ParsePorts does.
func ParsePublish(v string) (string, error) {
	l, err := ParsePorts(v)
	if err != nil || l == PortsAll || l == PortsNone {
		return "", msg.LinkPortsInvalid(strconv.Quote(v))
	}
	return l, nil
}

// ReadLinks returns a client VM's links, sorted by name, then port.
func ReadLinks(st *state.VM) ([]Link, error) {
	var out []Link
	for e := range strings.FieldsSeq(st.Get(KeyLinks)) {
		l, err := parseLink(e)
		if err != nil {
			return nil, fmt.Errorf("%s's links: %w", st.Name(), err)
		}
		out = append(out, l)
	}
	sortLinks(out)
	return out, nil
}

// parseLink reads one of formatLinks's entries:
// NAME,PORT,TARGET,TARGET_ID,ADDR, then ,group for one its group made.
func parseLink(e string) (Link, error) {
	f := strings.Split(e, ",")
	group := len(f) == 6 && f[5] == linkGroupMark
	if len(f) != 5 && !group {
		return Link{}, fmt.Errorf("%q isn't NAME,PORT,TARGET,ID,ADDR[,group]", e)
	}
	port, perr := ParseLinkPort(f[1])
	addr, aerr := netip.ParseAddr(f[4])
	switch {
	case CheckLinkName(f[0]) != nil:
		return Link{}, fmt.Errorf("%q: the name %q", e, f[0])
	case perr != nil:
		return Link{}, fmt.Errorf("%q: the port %q", e, f[1])
	case !vmname.IsVM(f[2]) || f[3] == "":
		return Link{}, fmt.Errorf("%q: the target %q, %q", e, f[2], f[3])
	case aerr != nil || !proxy.IsLinkAddr(addr) || addr != addr.Unmap():
		return Link{}, fmt.Errorf("%q: the address %q", e, f[4])
	}
	return Link{Name: f[0], Port: port, Target: f[2], TargetID: f[3], Addr: addr, Group: group}, nil
}

// formatLinks is links as KeyLinks holds them: space-separated entries,
// each formatLink's.
func formatLinks(links []Link) string {
	es := make([]string, len(links))
	for i, l := range links {
		es[i] = formatLink(l)
	}
	return strings.Join(es, " ")
}

// formatLink is one link's entry: NAME,PORT,TARGET,TARGET_ID,ADDR, then
// ,group when its group made it.
func formatLink(l Link) string {
	f := []string{l.Name, strconv.Itoa(l.Port), l.Target, l.TargetID, l.Addr.String()}
	if l.Group {
		f = append(f, linkGroupMark)
	}
	return strings.Join(f, ",")
}

func sortLinks(links []Link) {
	slices.SortFunc(links, func(a, b Link) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Port, b.Port)) })
}

// writeLinks records a client VM's links in its state.
func writeLinks(st *state.VM, links []Link) error {
	sortLinks(links)
	return st.Set(KeyLinks, formatLinks(links))
}

// linkAddr returns the client's address for a target (its VM name and
// ID): the one its links to that target have, else the lowest that none
// of its links has, from 127.77.0.1.
func linkAddr(links []Link, target, id string) (netip.Addr, error) {
	used := map[netip.Addr]bool{}
	for _, l := range links {
		if l.Target == target && l.TargetID == id {
			return l.Addr, nil
		}
		used[l.Addr] = true
	}
	for i := uint16(1); i < 0xffff; i++ {
		if a := proxy.LinkAddr(i); !used[a] {
			return a, nil
		}
	}
	return netip.Addr{}, errors.New("every address in 127.77.0.0/16 has a link")
}

// Why a link doesn't apply, as CheckLink and devvm link have it.
const (
	LinkGone        = "gone"        // the target has no state: deleted
	LinkReplaced    = "replaced"    // the target was made again: another ID
	LinkUnpublished = "unpublished" // the target doesn't publish the port
	LinkPolicy      = "policy"      // the client has network policy, the target doesn't
	LinkUngrouped   = "ungrouped"   // a group made it, and the two aren't in one group now
)

// grouped reports whether a link a group made still has that group: the
// client and the target are in the same one.
func grouped(client, target *state.VM) bool {
	g := client.Get(KeyGroup)
	return g != "" && target.Get(KeyGroup) == g
}

// CheckLink reports why a client's link doesn't apply, from the states
// as they are now ("" when it does), with the target's state: one that
// doesn't exist when it's gone.
func CheckLink(d paths.Dirs, client *state.VM, l Link) (string, *state.VM, error) {
	tst, err := state.ReadVM(d, l.Target)
	switch {
	case err != nil:
		return "", nil, err
	case !tst.Exists():
		return LinkGone, tst, nil
	case tst.Get(state.KeyID) != l.TargetID:
		return LinkReplaced, tst, nil
	case !Published(tst, l.Port):
		return LinkUnpublished, tst, nil
	case netpolicy.On(client) && !netpolicy.On(tst):
		return LinkPolicy, tst, nil
	case l.Group && !grouped(client, tst):
		return LinkUngrouped, tst, nil
	}
	return "", tst, nil
}

// Published reports whether a VM publishes port.
func Published(st *state.VM, port int) bool { return publishedIn(st.Get(KeyPublish), port) }

// publishedIn reports whether ports, as KeyPublish holds them, has port.
func publishedIn(ports string, port int) bool {
	if ports == "" || ports == PortsNone {
		return false
	}
	rs, err := backend.ParsePortRanges(ports)
	return err == nil && backend.Ports{Ranges: rs}.Has(port)
}

// linkFeatures are the backend features a link needs on both its VMs,
// checked with the registry's Require when it's made, at each launch of
// the client and on each connection.
func linkFeatures() []backend.Feature { return []backend.Feature{backend.FeatureLink} }

// PortTaken is the error for a link whose address and port a program in
// the client already listens on, which fails that link alone.
func (l Link) PortTaken(vm string) msg.M { return msg.LinkPortTaken(l.Name, strconv.Itoa(l.Port), vm) }

// Links shows and changes the folder's VM's links to other VMs, and the
// ports it publishes for theirs. Nothing here carries a connection: the
// client's link session does (Session), and changeLinks is where a
// running client learns of a change.
type Links struct{ *Env }

// linkVM finds the folder's VM for devvm link.
func (e Links) linkVM(ctx context.Context) (*found, error) { return e.folderVM(ctx, msg.NoVMLink) }

// changeLinks records client t's links and says so (said), then applies
// them while it runs: its setup sent again and its link session running
// (linkRuns.apply), which is waited for to say what came of added. A
// stopped client gets them at its next start.
func (e Links) changeLinks(ctx context.Context, t *found, links, added []Link, said func()) error {
	runs := linkRuns(e)
	from := runs.logSize(t.f.Name)
	if err := writeLinks(t.st, links); err != nil {
		return err
	}
	said()
	running, err := runs.apply(ctx, t.b, t.st, added, from)
	if err == nil && !running {
		e.Info(msg.LinksNextStart(t.f.Name))
	}
	return err
}

// publishChanged names the clients that link to ports of target t that it
// published before (old, as KeyPublish had it) and doesn't now: their next
// connections to those ports are refused.
func (e Links) publishChanged(t *found, old string) error {
	id := t.st.Get(state.KeyID)
	clients, err := linkClients(e.Dirs, t.f.Name, id, func(_ *state.VM, l Link) bool {
		return publishedIn(old, l.Port) && !Published(t.st, l.Port)
	})
	if err != nil || len(clients) == 0 {
		return err
	}
	e.Note(msg.UnpublishedLinked(t.f.Name, strings.Join(clients, ", ")))
	return nil
}

// publishUnenforced warns, for each client with network policy that links
// to ports of target t that it publishes now and didn't before (old, as
// KeyPublish had it), when t runs with its network policy not in force
// (warnUnenforced): those links' connections are refused until t starts
// again.
func (e Links) publishUnenforced(ctx context.Context, t *found, old string) error {
	clients, err := linkClients(e.Dirs, t.f.Name, t.st.Get(state.KeyID), func(c *state.VM, l Link) bool {
		return netpolicy.On(c) && !publishedIn(old, l.Port) && Published(t.st, l.Port)
	})
	if err != nil {
		return err
	}
	for _, vm := range clients {
		cst, err := state.ReadVM(e.Dirs, vm)
		if err != nil {
			return err
		}
		e.warnUnenforced(ctx, cst, t.st)
	}
	return nil
}

// projectVMs returns the project VMs that have state.
func (e Links) projectVMs() ([]string, error) {
	names, err := state.VMNames(e.Dirs)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(names, func(n string) bool { return !vmname.IsVM(n) }), nil
}

// resolve returns the VM a link's TARGET names: a folder's path (relative
// to the folder devvm runs in), a VM's name, or a folder's name, which
// must match one VM alone.
func (e Links) resolve(target string) (string, error) {
	all, err := e.projectVMs()
	if err != nil {
		return "", err
	}
	var vms []string
	if project.IsPath(target) {
		dir := target
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(e.Dir, dir)
		}
		if dir, err = filepath.EvalSymlinks(dir); err == nil {
			vms = project.ForDir(dir, all)
		}
	} else {
		vms = project.Targets(target, all)
	}
	switch len(vms) {
	case 0:
		return "", msg.LinkUnknownTarget(target)
	case 1:
		return vms[0], nil
	}
	shown := make([]string, len(vms))
	for i, vm := range vms {
		dir := "-"
		if st, err := state.ReadVM(e.Dirs, vm); err == nil && st.Get(state.KeyDir) != "" {
			dir = st.Get(state.KeyDir)
		}
		shown[i] = vm + " in " + dir
	}
	return "", msg.LinkAmbiguousTarget(target, strings.Join(shown, ", "))
}

// LinkListed is one of the folder's VM's links as devvm link lists it.
type LinkListed struct {
	Name, Target string
	Port         int
	Folder       string // the target's folder; "" once it's gone
	Status       string // the target's, as its backend reports it; "" when unknown
	Reason       string // why it doesn't apply (CheckLink); "" when it does
}

// List returns the folder's VM's links, with each target's folder and
// status and whether the link applies.
func (e Links) List(ctx context.Context) ([]LinkListed, error) {
	t, err := e.linkVM(ctx)
	if err != nil {
		return nil, err
	}
	links, err := ReadLinks(t.st)
	if err != nil {
		return nil, err
	}
	status := map[string]string{}
	out := make([]LinkListed, 0, len(links))
	for _, l := range links {
		why, tst, err := CheckLink(e.Dirs, t.st, l)
		if err != nil {
			return nil, err
		}
		ll := LinkListed{Name: l.Name, Target: l.Target, Port: l.Port, Reason: why, Folder: tst.Get(state.KeyDir)}
		if tst.Exists() {
			s, ok := status[l.Target]
			if !ok {
				s = e.targetStatus(ctx, tst)
				status[l.Target] = s
			}
			ll.Status = s
		}
		out = append(out, ll)
	}
	return out, nil
}

// targetStatus is a target's status as its backend reports it: "" when it
// can't say, or doesn't have the VM.
func (e Links) targetStatus(ctx context.Context, tst *state.VM) string {
	c, err := e.Registry.For(e.Dirs, tst, "")
	if err != nil {
		return ""
	}
	in, err := c.Backend.Inspect(ctx, tst.Name())
	if err != nil {
		return ""
	}
	return string(in.Status)
}

// LinkAdd is what devvm link add makes: a link to TARGET's Port, named As
// ("" for the target's folder part).
type LinkAdd struct {
	Target, As string
	Port       int
}

// Add makes a link from the folder's VM to a's target, once it's checked:
// the target is another project VM that publishes the port, network
// policy on the client means network policy on the target, both backends
// have links, and the name is free. A running target whose network policy
// isn't in force is warned of (warnUnenforced), and refused nothing.
func (e Links) Add(ctx context.Context, a LinkAdd) error {
	t, err := e.linkVM(ctx)
	if err != nil {
		return err
	}
	client := t.f.Name
	target, err := e.resolve(a.Target)
	if err != nil {
		return err
	}
	if target == client {
		return msg.LinkSelf(client)
	}
	tst, err := e.checkTarget(t, target, a.Port)
	if err != nil {
		return err
	}
	links, err := ReadLinks(t.st)
	if err != nil {
		return err
	}
	// The user's own links decide the name; the group's follow it.
	own := ownLinks(links)
	id := tst.Get(state.KeyID)
	name, err := linkNameFor(t, own, target, id, cmp.Or(a.As, linkedName(links, target, id)))
	if err != nil {
		return err
	}
	port := strconv.Itoa(a.Port)
	if slices.ContainsFunc(own, func(l Link) bool { return l.Target == target && l.TargetID == id && l.Port == a.Port }) {
		e.Info(msg.LinkExists(name, port, client))
		e.warnUnenforced(ctx, t.st, tst)
		return nil
	}
	if ok, err := t.widen(e.Env, "devvm link add"); !ok {
		return err
	}
	addr, err := linkAddr(links, target, id)
	if err != nil {
		return err
	}
	l := Link{Name: name, Port: a.Port, Target: target, TargetID: id, Addr: addr}
	all, clashes, err := Groups(e).withGroup(t.st, append(own, l), links)
	if err != nil {
		return err
	}
	// Only the clash this link makes: the group's links that its name
	// takes from another member.
	clashes = slices.DeleteFunc(clashes, func(c groupClash) bool { return c.Name != name })
	return e.changeLinks(ctx, t, all, []Link{l}, func() {
		e.Info(msg.LinkAdded(name, port, target, client))
		e.Note(msg.LinkTrust(target, client))
		e.warnUnenforced(ctx, t.st, tst)
		Groups(e).warnClashes(client, clashes)
		Groups(e).sayChanged(t.st, links, all, nil)
	})
}

// checkTarget checks a new link's target for client t: made and finished,
// publishing port, with network policy if the client has it, and on a
// backend with links, as the client's is. It returns the target's state.
func (e Links) checkTarget(t *found, target string, port int) (*state.VM, error) {
	tst, err := state.ReadVM(e.Dirs, target)
	if err != nil {
		return nil, err
	}
	if tst.Get(state.KeyID) == "" || tst.Get(KeyNet) == "" {
		return nil, msg.VMUnfinished(target)
	}
	if !Published(tst, port) {
		return nil, msg.LinkNotPublished(strconv.Itoa(port), target)
	}
	if netpolicy.On(t.st) && !netpolicy.On(tst) {
		return nil, msg.LinkPolicyMismatch(target, t.f.Name)
	}
	c, err := e.Registry.For(e.Dirs, tst, "")
	if err != nil {
		return nil, err
	}
	if err := e.Registry.Require(t.b, t.f.Name, linkFeatures()...); err != nil {
		return nil, err
	}
	if err := e.Registry.Require(c.Backend, target, linkFeatures()...); err != nil {
		return nil, err
	}
	return tst, nil
}

// linkNameFor returns the name of client t's new link to target (with
// its ID): as, else the name its links to that target have, else the
// target's folder part. A target has one name in a client, and a name
// one target; localhost and the client's own hostname are refused.
func linkNameFor(t *found, links []Link, target, id, as string) (string, error) {
	had := ""
	for _, l := range links {
		if l.Target == target && l.TargetID == id {
			had = l.Name
		}
	}
	name := cmp.Or(as, had, (project.Folder{Name: target}).Slug())
	if had != "" && name != had {
		return "", msg.LinkTargetNamed(had, target, t.f.Name)
	}
	if err := CheckLinkName(name); err != nil {
		return "", err
	}
	if name == "localhost" || name == t.f.Slug() {
		return "", msg.LinkNameHost(name, t.f.Name)
	}
	for _, l := range links {
		if l.Name == name && (l.Target != target || l.TargetID != id) {
			return "", msg.LinkNameTaken(name, l.Target, t.f.Name)
		}
	}
	return name, nil
}

// Rm removes the folder's VM's link to target's port, where target is the
// link's name, its target VM's name, or what devvm link add takes.
func (e Links) Rm(ctx context.Context, target string, port int) error {
	t, err := e.linkVM(ctx)
	if err != nil {
		return err
	}
	links, err := ReadLinks(t.st)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(links, func(l Link) bool { return l.Port == port && l.Name == target })
	if i < 0 {
		i = slices.IndexFunc(links, func(l Link) bool { return l.Port == port && l.Target == target })
	}
	if i < 0 {
		if vm, err := e.resolve(target); err == nil {
			i = slices.IndexFunc(links, func(l Link) bool { return l.Port == port && l.Target == vm })
		}
	}
	if i < 0 {
		return msg.LinkNotFound(strconv.Itoa(port), target, t.f.Name)
	}
	gone := links[i]
	if gone.Group {
		// One left from a group the two aren't in together, or to a VM
		// that's gone or made again, goes like any other.
		why, _, err := CheckLink(e.Dirs, t.st, gone)
		if err != nil {
			return err
		}
		if why != LinkGone && why != LinkReplaced && why != LinkUngrouped {
			return msg.LinkFromGroup(t.st.Get(KeyGroup), gone.Name, strconv.Itoa(port), t.f.Name)
		}
	}
	// The group's links follow: a member's port the user linked gets the
	// group's link again.
	all, _, err := Groups(e).withGroup(t.st, ownLinks(slices.Delete(slices.Clone(links), i, i+1)), links)
	if err != nil {
		return err
	}
	regained := slices.DeleteFunc(slices.Clone(all), func(l Link) bool {
		return !l.Group || l.Target != gone.Target || l.Port != gone.Port
	})
	return e.changeLinks(ctx, t, all, regained, func() {
		e.Info(msg.LinkRemoved(gone.Name, strconv.Itoa(port), t.f.Name))
		Groups(e).sayChanged(t.st, links, all, regained)
	})
}

// Published returns the ports and ranges the folder's VM publishes, or
// none.
func (e Links) Published(ctx context.Context) (string, error) {
	t, err := e.linkVM(ctx)
	if err != nil {
		return "", err
	}
	return cmp.Or(t.st.Get(KeyPublish), PortsNone), nil
}

// Publish offers list's ports and ranges (ParsePublish) to links from the
// user's other VMs, or with publish false, withdraws them, each as
// listed.
func (e Links) Publish(ctx context.Context, list string, publish bool) error {
	l, err := ParsePublish(list)
	if err != nil {
		return err
	}
	t, err := e.linkVM(ctx)
	if err != nil {
		return err
	}
	old := cmp.Or(t.st.Get(KeyPublish), PortsNone)
	var ports string
	if publish {
		if ok, err := t.widen(e.Env, "devvm link publish"); !ok {
			return err
		}
		ports, err = AddPorts(old, l)
	} else {
		ports, err = dropPorts(old, l, msg.PortNotPublished)
	}
	if err != nil {
		return err
	}
	stored := ports
	if ports == PortsNone {
		stored = ""
	}
	if err := t.st.Set(KeyPublish, stored); err != nil {
		return err
	}
	// The group's other members get links to the ports at once, or lose
	// them, before the clients with links of their own are named.
	if publish {
		e.Info(msg.Published(ports, t.f.Name))
		if err := e.publishUnenforced(ctx, t, old); err != nil {
			return err
		}
		return Groups(e).syncOthers(ctx, t)
	}
	e.Info(msg.Unpublished(l, t.f.Name))
	if err := Groups(e).syncOthers(ctx, t); err != nil {
		return err
	}
	return e.publishChanged(t, old)
}
