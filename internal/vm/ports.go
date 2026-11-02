package vm

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// Ports values: every port the VM listens on is forwarded, or none.
const (
	PortsAll  = "all"
	PortsNone = "none"
)

// ParsePorts checks which ports to forward, as --ports or the ports file
// give them: all, none, or ports and ranges (5173,8000-8010,
// backend.ParsePortRanges). It returns the value without spaces or
// repeats.
func ParsePorts(v string) (string, error) {
	if s := strings.ReplaceAll(v, " ", ""); s == PortsAll || s == PortsNone {
		return s, nil
	}
	rs, err := backend.ParsePortRanges(v)
	if err != nil {
		return "", msg.PortInvalid(strconv.Quote(v)).Because(err)
	}
	return backend.FormatPortRanges(rs), nil
}

// ParsePortList checks a list of ports and ranges, as devvm ports add and
// rm take it.
func ParsePortList(list string) (string, error) {
	l, err := ParsePorts(list)
	if err != nil {
		return "", err
	}
	if l == PortsAll || l == PortsNone {
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
	case old == PortsAll:
		return PortsAll, nil
	case old == PortsNone:
		return l, nil
	}
	return ParsePorts(old + "," + l)
}

// RemovePorts returns a parsed ports value without list's ports and
// ranges, each as listed in it: none once nothing is left.
func RemovePorts(old, list string) (string, error) {
	if old == PortsAll {
		return "", msg.PortsRmAll()
	}
	l, err := ParsePortList(list)
	if err != nil {
		return "", err
	}
	if old, err = ParsePorts(old); err != nil {
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
		return PortsNone, nil
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
		if old == PortsAll {
			warn(msg.PortsAllAlready())
		}
		return AddPorts(old, c.List)
	case "rm":
		return RemovePorts(old, c.List)
	}
	return ParsePorts(c.List)
}

// defaultPorts reads what new VMs forward, checked: the file may have
// been edited by hand.
func (e *Env) defaultPorts() (string, error) {
	def, err := state.ReadPorts(e.Dirs)
	if err != nil || def == "" {
		return PortsAll, err
	}
	return ParsePorts(def)
}

func orAll(ports string) string {
	if ports == "" {
		return PortsAll
	}
	return ports
}

// portsVM finds the folder's VM for devvm ports.
func (e Settings) portsVM(ctx context.Context) (*found, error) { return e.folderVM(ctx, msg.NoVMPorts) }

// Ports returns which ports the folder's VM forwards (all, none or a
// list); with global, what new VMs forward.
func (e Settings) Ports(ctx context.Context, global bool) (string, error) {
	if global {
		return e.defaultPorts()
	}
	t, err := e.portsVM(ctx)
	if err != nil {
		return "", err
	}
	return orAll(t.st.Get(KeyPorts)), nil
}

// SetPorts changes which ports the folder's VM forwards: a stopped VM's
// from its next start, a running one's at once, its forwarder started
// again while it runs on (forwarding). With c.Global, it changes what new
// VMs forward instead.
func (e Settings) SetPorts(ctx context.Context, c PortsChange) error {
	if c.Global {
		old, err := e.defaultPorts()
		if err != nil {
			return err
		}
		ports, err := c.apply(old, e.Warn)
		if err != nil {
			return err
		}
		if err := state.WritePorts(e.Dirs, ports); err != nil {
			return err
		}
		e.Info(msg.PortsDefault(ports))
		return nil
	}
	t, err := e.portsVM(ctx)
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	ports, err := c.apply(orAll(st.Get(KeyPorts)), e.Warn)
	if err != nil {
		return err
	}
	p, err := PortsBackend(ports)
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
	// (netpolicy.UsedPorts), its new ones included before its forwarder
	// starts with them. Put back when its backend fails.
	old := st.Get(KeyPorts)
	if err := st.Set(KeyPorts, ports); err != nil {
		return err
	}
	if err := b.SetPorts(ctx, vm, p, conf); err != nil {
		_ = st.Set(KeyPorts, old)
		return err
	}
	if running {
		e.Info(msg.PortsForwarding(ports, vm))
	} else {
		e.Info(msg.PortsNow(ports, vm))
	}
	return nil
}

// forwarding is what a running VM's forwarder starts again with when its
// ports change: the host key its ssh config pins, its proxy's and
// decider's ports, never forwarded, and its listed ports less those a host
// program has (freePorts).
func (e *Env) forwarding(b backend.Backend, st *state.VM) backend.Confinement {
	vm := st.Name()
	c := backend.Confinement{
		Free:    e.freePorts(vm),
		HostKey: func(ctx context.Context) (string, error) { return e.sshConfig().HostKey(ctx, b, vm) },
	}
	c.ProxyPort, _ = strconv.Atoi(st.Get(netpolicy.KeyProxyPort))
	if p := st.Get(netpolicy.KeyDeciderPort); p != "" {
		c.Decider = "127.0.0.1:" + p
	}
	return c
}

// PortsBackend returns a ports value as the backend has it: "" is all,
// Lima's default. A value that isn't one (a state file edited by hand)
// is port_invalid.
func PortsBackend(v string) (backend.Ports, error) {
	switch v {
	case "", PortsAll:
		return backend.Ports{All: true}, nil
	case PortsNone:
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
