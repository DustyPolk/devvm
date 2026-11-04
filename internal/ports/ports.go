// Package ports keeps a VM's ports section of its state: the ports it
// forwards, as the user asked, and the host ports devvm chose for it (its
// ssh's, its proxy's, its desktop's). One allocator chooses every host
// port devvm chooses, under the claims lock (state.VM.Claim), so no two
// VMs, and no VM's forwards, get one.
package ports

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
)

// Section is the name of the ports section in a VM's state.
const Section = "ports"

// Ports is a VM's ports section.
type Ports struct {
	// Forward is the ports it forwards to the same host port, as asked
	// (devvm ports, --ports), as settings.ParsePorts returns them: "" for
	// all.
	Forward string `json:"forward,omitempty"`
	// Host is the host ports devvm chose for it.
	Host Host `json:"host,omitzero"`
}

// Host is the host ports devvm chose for a VM: 0 for none yet.
type Host struct {
	SSH     int `json:"ssh,omitempty"`     // its ssh's, which devvm chooses so no forwarder takes it first
	Proxy   int `json:"proxy,omitempty"`   // its network policy proxy's, on the host's loopback
	Desktop int `json:"desktop,omitempty"` // devvm desktop's, on the host's loopback
}

// Of returns a VM's ports section.
func Of(st *state.VM) (Ports, error) { return state.Section[Ports](st, Section) }

// Set sets a VM's ports section, in st alone: inside st.Update.
func Set(st *state.VM, p Ports) error { return state.SetSection(st, Section, p) }

// SetForward sets which ports a VM forwards (Ports.Forward), in st alone.
func SetForward(st *state.VM, forward string) error {
	p, err := Of(st)
	if err != nil {
		return err
	}
	p.Forward = forward
	return Set(st, p)
}

// Forwarding returns which ports a VM forwards, as a ports value
// (Ports.Forward, settings.ParsePorts) says: "" and all are every port,
// none is none. A value that isn't one (a state file edited by hand) is
// port_invalid, so a bad entry never becomes a port the VM didn't ask
// for.
func Forwarding(v string) (backend.Ports, error) {
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

// Forwarded returns the host ports a VM forwards, as p lists them
// (5173,8000-8010, backend.ParsePortRanges), each entry on its own, so
// one that isn't a port or a range is skipped and the others are still
// avoided; none for all or none, which name no port.
func (p Ports) Forwarded() map[int]bool {
	out := map[int]bool{}
	for e := range strings.SplitSeq(p.Forward, ",") {
		rs, err := backend.ParsePortRanges(e)
		if err != nil {
			continue
		}
		for n := rs[0].From; n <= rs[0].To; n++ {
			out[n] = true
		}
	}
	return out
}

// Used returns the host ports VMs have (ssh, proxy, desktop), and the
// ports they list to forward, but self's (a VM's name, or ""): what a new
// one avoids, and what a VM's port forwarder never takes, self being that
// VM.
func Used(d paths.Dirs, self string) (map[int]bool, error) {
	names, err := state.VMNames(d)
	if err != nil {
		return nil, err
	}
	used := map[int]bool{}
	for _, name := range names {
		st, err := state.ReadVM(d, name)
		if err != nil {
			continue // not a VM's state
		}
		p, err := Of(st)
		if err != nil {
			continue
		}
		for _, n := range []int{p.Host.SSH, p.Host.Proxy, p.Host.Desktop} {
			if n != 0 {
				used[n] = true
			}
		}
		if name == self {
			continue
		}
		for n := range p.Forwarded() {
			used[n] = true
		}
	}
	return used, nil
}

// Allocator chooses the host ports devvm chooses for VMs.
type Allocator struct {
	Dirs paths.Dirs
	// Listening reports whether a program has a host loopback port
	// (Listening).
	Listening func(ctx context.Context, port int) bool
	// Range is the range new ports come from (platform.ChoosablePorts,
	// which the zero value means too).
	Range platform.PortRange
}

// Which is one of a VM's host ports, as Choose takes it.
type Which func(*Host) *int

// SSH is a VM's ssh port, as Choose takes it.
func SSH(h *Host) *int { return &h.SSH }

// Proxy is a VM's proxy port, as Choose takes it.
func Proxy(h *Host) *int { return &h.Proxy }

// Desktop is a VM's desktop port, as Choose takes it.
func Desktop(h *Host) *int { return &h.Desktop }

// Choose returns the VM's host port which: the one its state records
// while keep says so (given that port and the VM's ports as they are
// now), else a new one, chosen (free) and recorded in st under the
// claims lock, so two devvm runs never choose the same.
func (a Allocator) Choose(ctx context.Context, st *state.VM, which Which, keep func(int, Ports) bool) (int, error) {
	var n int
	err := st.Claim(ctx, func(st *state.VM) error {
		p, err := Of(st)
		if err != nil {
			return err
		}
		if cur := *which(&p.Host); cur != 0 && keep(cur, p) {
			n = cur
			return nil
		}
		if n, err = a.free(ctx); err != nil {
			return err
		}
		*which(&p.Host) = n
		return Set(st, p)
	})
	return n, err
}

// SSH returns the host port the VM's ssh is forwarded from, with or
// without network policy, as its state records it: devvm chooses it,
// rather than the backend at its start, so every VM's port forwarder
// avoids it (Used) before the VM's backend binds it. A stopped VM gets
// a new one when its port is now another program's, on any address, or
// one of its own forwards, which its backend would refuse.
func (a Allocator) SSH(ctx context.Context, st *state.VM, running bool) (int, error) {
	if p, err := Of(st); err == nil && p.Host.SSH != 0 && running {
		return p.Host.SSH, nil
	}
	return a.Choose(ctx, st, SSH, func(port int, p Ports) bool {
		return running || !p.Forwarded()[port] && !a.Listening(ctx, port)
	})
}

// free returns a loopback port of Range (below the host's ephemeral
// range) no program has (Listening) and no VM has.
func (a Allocator) free(ctx context.Context) (int, error) {
	used, err := Used(a.Dirs, "")
	if err != nil {
		return 0, err
	}
	r := a.Range
	if r == (platform.PortRange{}) {
		r = platform.ChoosablePorts()
	}
	return platform.FreePort(ctx, r, func(ctx context.Context, port int) bool {
		return !used[port] && !a.Listening(ctx, port)
	})
}

// Listening reports whether a program has a host loopback port: one that
// has it on any address (platform.PortInUse), even if it never accepts,
// or one that accepts connections on it.
func Listening(ctx context.Context, port int) bool {
	if platform.PortInUse(ctx, port) {
		return true
	}
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
