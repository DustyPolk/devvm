package vm

import (
	"context"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// Ports values: every port the VM listens on is forwarded, or none.
const (
	PortsAll  = "all"
	PortsNone = "none"
)

var portEntry = regexp.MustCompile(`^([0-9]{1,5})(-([0-9]{1,5}))?$`)

// ParsePorts checks which ports to forward, as --ports or the ports file
// give them: all, none, or ports and ranges (5173,8000-8010). It returns
// the value without spaces or repeats.
func ParsePorts(v string) (string, error) {
	s := strings.ReplaceAll(v, " ", "")
	if s == PortsAll || s == PortsNone {
		return s, nil
	}
	bad := msg.PortInvalid(strconv.Quote(v))
	if s == "" {
		return "", bad
	}
	var out []string
	for e := range strings.SplitSeq(s, ",") {
		m := portEntry.FindStringSubmatch(e)
		if m == nil {
			return "", bad
		}
		a, _ := strconv.Atoi(m[1])
		b := a
		if m[3] != "" {
			b, _ = strconv.Atoi(m[3])
		}
		if a < 1 || a > b || b > 65535 {
			return "", bad
		}
		e = strconv.Itoa(a)
		if a != b {
			e += "-" + strconv.Itoa(b)
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return strings.Join(out, ","), nil
}

// parseList checks a list of ports and ranges, as add and rm take it.
func parseList(list string) (string, error) {
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
	l, err := parseList(list)
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
	l, err := parseList(list)
	if err != nil {
		return "", err
	}
	es := strings.Split(old, ",")
	for e := range strings.SplitSeq(l, ",") {
		i := slices.Index(es, e)
		if i < 0 {
			return "", msg.PortNotListed(e, old)
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

// portsVM finds the folder's VM for devvm ports.
func (e *Env) portsVM(ctx context.Context) (*found, error) { return e.folderVM(ctx, msg.NoVMPorts) }

// Ports writes to w which ports the folder's VM forwards (all, none or a
// list), saying so when that changes at its next start; with global,
// what new VMs forward.
func (e *Env) Ports(ctx context.Context, w io.Writer, global bool) error {
	if global {
		def, err := e.defaultPorts()
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, def+"\n")
		return err
	}
	t, err := e.portsVM(ctx)
	if err != nil {
		return err
	}
	want, set := orAll(t.st.Get(KeyPorts)), orAll(t.st.Get(KeyPortsSet))
	if _, err := io.WriteString(w, set+"\n"); err != nil {
		return err
	}
	if want != set {
		e.Note(msg.PortsPending(want, t.f.Name))
	}
	return nil
}

// SetPorts changes which ports the folder's VM forwards: at once when
// it's stopped, from its next start when it runs and forwards others.
// With c.Global, it
// changes what new VMs forward instead.
func (e *Env) SetPorts(ctx context.Context, c PortsChange) error {
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
	if err := backend.Require(b, vm, PortsBackend(ports).Features()...); err != nil {
		return err
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if err := st.Set(KeyPorts, ports); err != nil {
		return err
	}
	running := in.Status == backend.StatusRunning
	if err := e.applyPorts(ctx, b, st, running); err != nil {
		return err
	}
	// Otherwise applyPorts said it waits for the next start.
	if orAll(st.Get(KeyPortsSet)) == ports {
		e.Info(msg.PortsNow(ports, vm))
	}
	return nil
}

// PortsBackend returns a parsed ports value as the backend has it: "" is
// all, Lima's default.
func PortsBackend(v string) backend.Ports {
	switch v {
	case "", PortsAll:
		return backend.Ports{All: true}
	case PortsNone:
		return backend.Ports{}
	}
	var p backend.Ports
	for e := range strings.SplitSeq(v, ",") {
		a, b, ok := strings.Cut(e, "-")
		from, _ := strconv.Atoi(a)
		to := from
		if ok {
			to, _ = strconv.Atoi(b)
		}
		p.Ranges = append(p.Ranges, backend.PortRange{From: from, To: to})
	}
	return p
}
