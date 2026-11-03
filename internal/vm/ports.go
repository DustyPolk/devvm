package vm

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
)

// ParsePortList checks a list of ports and ranges, as devvm ports add and
// rm take it.
func ParsePortList(list string) (string, error) {
	l, err := settings.ParsePorts(list)
	if err != nil {
		return "", err
	}
	if l == settings.PortsAll || l == settings.PortsNone {
		return "", msg.PortsListNeeded(strconv.Quote(list))
	}
	return l, nil
}

// AddPorts returns a parsed ports value with list's ports and ranges
// added: all stays all.
func AddPorts(old, list string) (string, error) {
	l, err := ParsePortList(list)
	switch {
	case err != nil:
		return "", err
	case old == settings.PortsAll:
		return settings.PortsAll, nil
	case old == settings.PortsNone:
		return l, nil
	}
	return settings.ParsePorts(old + "," + l)
}

// RemovePorts returns a parsed ports value without list's ports and
// ranges, each as listed in it: none once nothing is left.
func RemovePorts(old, list string) (string, error) {
	if old == settings.PortsAll {
		return "", msg.PortsRmAll()
	}
	l, err := ParsePortList(list)
	if err != nil {
		return "", err
	}
	if old, err = settings.ParsePorts(old); err != nil {
		return "", err
	}
	return dropPorts(old, l, msg.PortNotListed)
}

// dropPorts returns a parsed list of ports and ranges, old, without those
// of l, a parsed list, each as listed in old (else notListed): none once
// nothing is left.
func dropPorts(old, l string, notListed func(port, ports string) msg.M) (string, error) {
	es := strings.Split(old, ",")
	for e := range strings.SplitSeq(l, ",") {
		i := slices.Index(es, e)
		if i < 0 {
			return "", notListed(e, old)
		}
		es = slices.Delete(es, i, i+1)
	}
	if len(es) == 0 {
		return settings.PortsNone, nil
	}
	return strings.Join(es, ","), nil
}

// PortsChange is a change devvm ports makes: Op is "" (set to List),
// "add" or "rm".
type PortsChange struct {
	Op, List string
	Global   bool // what new VMs forward instead
}

// apply returns old changed by c, warning through warn when it adds to
// all.
func (c PortsChange) apply(old string, warn func(msg.M)) (string, error) {
	switch c.Op {
	case "add":
		if old == settings.PortsAll {
			warn(msg.PortsAllAlready())
		}
		return AddPorts(old, c.List)
	case "rm":
		return RemovePorts(old, c.List)
	}
	return settings.ParsePorts(c.List)
}

func orAll(ports string) string {
	if ports == "" {
		return settings.PortsAll
	}
	return ports
}

// portsVM finds the folder's VM for devvm ports.
func (e Settings) portsVM(ctx context.Context) (*found, error) { return e.folderVM(ctx, msg.NoVMPorts) }

// Ports returns which ports the folder's VM forwards: all, none or a
// list.
func (e Settings) Ports(ctx context.Context) (string, error) {
	t, err := e.portsVM(ctx)
	if err != nil {
		return "", err
	}
	return orAll(sectionOf[ports.Ports](t.st, ports.Section).Forward), nil
}

// PortsDefault returns what new VMs forward, and where that came from.
func (e Settings) PortsDefault() (settings.Value, error) {
	def, err := settings.Read(e.Dirs)
	return def.Ports, err
}

// SetPorts changes which ports the folder's VM forwards: a stopped VM's
// from its next start, a running one's at once, its forwarder started
// again while it runs on (forwarding). With c.Global, it changes what new
// VMs forward instead, which config.toml mustn't set.
func (e Settings) SetPorts(ctx context.Context, c PortsChange) error {
	if c.Global {
		old, err := e.PortsDefault()
		if err != nil {
			return err
		}
		list, err := c.apply(old.Value, e.Warn)
		if err != nil {
			return err
		}
		if _, err := settings.Save(e.Dirs, settings.Values{Ports: &list}); err != nil {
			return err
		}
		e.Info(msg.PortsDefault(list))
		return nil
	}
	t, err := e.portsVM(ctx)
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	cur, err := ports.Of(st)
	if err != nil {
		return err
	}
	list, err := c.apply(orAll(cur.Forward), e.Warn)
	if err != nil {
		return err
	}
	p, err := PortsBackend(list)
	if err != nil {
		return err
	}
	if err := e.Registry.Require(b, vm, p.Features()...); err != nil {
		return err
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	running := in.Status == backend.StatusRunning
	var conf backend.Confinement
	if running {
		conf = e.forwarding(b, st)
	}
	// Its state first: other VMs' forwarders never take its listed ports
	// (ports.Used), its new ones included before its forwarder starts with
	// them. Put back when its backend fails.
	if err := st.Update(ctx, func(st *state.VM) error { return ports.SetForward(st, list) }); err != nil {
		return err
	}
	if err := b.SetPorts(ctx, vm, p, conf); err != nil {
		_ = st.Update(ctx, func(st *state.VM) error { return ports.SetForward(st, cur.Forward) })
		return err
	}
	if running {
		e.Info(msg.PortsForwarding(list, vm))
	} else {
		e.Info(msg.PortsNow(list, vm))
	}
	return nil
}

// forwarding is what a running VM's forwarder starts again with when its
// ports change: the host key its ssh config pins, its proxy's port, never
// forwarded, and its listed ports less those a host program has
// (freePorts).
func (e *Env) forwarding(b backend.Backend, st *state.VM) backend.Confinement {
	vm := st.Name()
	c := backend.Confinement{
		Free:    e.freePorts(vm),
		HostKey: func(ctx context.Context) (string, error) { return e.sshConfig().HostKey(ctx, b, vm) },
	}
	c.ProxyPort = sectionOf[ports.Ports](st, ports.Section).Host.Proxy
	return c
}

// PortsBackend returns a ports value as the backend has it: "" is all,
// Lima's default. A value that isn't one (a state file edited by hand)
// is port_invalid.
func PortsBackend(v string) (backend.Ports, error) {
	switch v {
	case "", settings.PortsAll:
		return backend.Ports{All: true}, nil
	case settings.PortsNone:
		return backend.Ports{}, nil
	}
	rs, err := backend.ParsePortRanges(v)
	if err != nil {
		return backend.Ports{}, msg.PortInvalid(strconv.Quote(v)).Because(err)
	}
	return backend.Ports{Ranges: rs}, nil
}

// freePorts is the VM's Confinement.Free: its listed ports less those a
// program on the host has on any address, which a forward would share,
// taking that program's loopback connections. They're left out, warned,
// until a start or a change of its ports finds them free.
func (e *Env) freePorts(vm string) func(context.Context, []backend.PortRange) []backend.PortRange {
	return func(ctx context.Context, listed []backend.PortRange) []backend.PortRange {
		free, taken := splitPorts(listed, func(p int) bool { return e.PortInUse(ctx, p) })
		if len(taken) > 0 {
			n := 0
			for _, r := range taken {
				n += r.To - r.From + 1
			}
			e.Warn(msg.PortsTaken(n, backend.FormatPortRanges(taken), vm))
		}
		return free
	}
}

// splitPorts splits ranges into the ports inUse says are free and those it
// says aren't, each as ranges: a range wholly free stays as it is.
func splitPorts(rs []backend.PortRange, inUse func(int) bool) (free, taken []backend.PortRange) {
	add := func(out []backend.PortRange, from, p int) []backend.PortRange {
		if n := len(out); n > 0 && out[n-1].From >= from && out[n-1].To == p-1 {
			out[n-1].To = p
			return out
		}
		return append(out, backend.PortRange{From: p, To: p})
	}
	for _, r := range rs {
		for p := r.From; p <= r.To; p++ {
			if inUse(p) {
				taken = add(taken, r.From, p)
			} else {
				free = add(free, r.From, p)
			}
		}
	}
	return free, taken
}
