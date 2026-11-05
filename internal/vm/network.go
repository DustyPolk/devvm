package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Net shows and changes the folder's VM's network policy, its network log
// and its allowlist (here), and the secrets its proxy puts in
// (secret.go).
type Net struct{ *Env }

// NetworkShown is the folder's VM's network policy, as devvm network
// shows it.
type NetworkShown struct {
	Policy string // on, off or unenforced, as devvm ls shows it
	// Rules are the rules of each policy file its proxy reads that has
	// any, in order.
	Rules []netpolicy.RuleFile
}

// Network returns the folder's VM's network policy and its allowlist.
func (e Net) Network(ctx context.Context) (NetworkShown, error) {
	var s NetworkShown
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return s, err
	}
	in, err := t.b.Inspect(ctx, t.f.Name)
	if err != nil {
		return s, err
	}
	if s.Policy, err = netpolicy.Listed(ctx, t.b, t.st, in.Status == backend.StatusRunning); err != nil {
		return s, err
	}
	builtin, files := netpolicy.Policies(e.Dirs, t.f.Name, image.Name(t.st.Image), tools.Names(t.st))
	s.Rules, err = netpolicy.ReadRules(e.Proxy.Assets, builtin, files)
	return s, err
}

// NetworkDefault returns whether new VMs get network policy (ask, on or
// off), and where that came from.
func (e Net) NetworkDefault() (settings.Value, error) {
	def, err := settings.Read(e.Dirs)
	return def.Network, err
}

// SetNetworkDefault saves whether new VMs get network policy, as devvm
// setup does (NetworkChoice).
func (e Net) SetNetworkDefault(mode string) error {
	return NetworkChoice{Dirs: e.Dirs, Info: e.Info}.Set(mode)
}

// NetworkChoice chooses whether new VMs get network policy, which devvm
// network -g and devvm setup save in settings.json. The flags of devvm
// still choose for one VM.
type NetworkChoice struct {
	Dirs paths.Dirs
	Info func(msg.M) // says what's saved
}

// Set saves mode (a settings.NetworkChoices value) as new VMs' network
// policy; config.toml mustn't set it (set_in_config).
func (c NetworkChoice) Set(mode string) error {
	if _, err := settings.Save(c.Dirs, settings.Values{Network: &mode}); err != nil {
		return err
	}
	c.Info(msg.NetworkDefault(mode))
	return nil
}

// SetNetwork turns the folder's VM's network policy on or off, from its
// next start: a running VM's restarts now once the user agrees
// (restartNow). Turning it on, its backend must be able to confine the VM
// to its proxy, and tokens the VM got before stay valid. Either is
// refused while it would break a link's policy rule (policyLinked), and
// while the VM is in a group, whose VMs all have network policy or none
// do.
func (e Net) SetNetwork(ctx context.Context, on bool) error {
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	to := "off"
	if on {
		to = "on"
	}
	if netpolicy.On(st) == on {
		e.Info(msg.NetAlready(to, vm))
		return nil
	}
	// A group's members all have network policy or none do.
	if g := groupOf(st); g != "" {
		return msg.GroupPolicyMixed(g, vm)
	}
	// Links' policy rule: a client with network policy links only to a
	// target with it.
	if err := policyLinked(e.Dirs, st, on); err != nil {
		return err
	}
	if !on {
		if ok, err := t.widen(e.Env, "devvm network off"); !ok {
			return err
		}
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	running := in.Status == backend.StatusRunning
	if on {
		if err := e.Registry.Require(b, vm, backend.Confinement{Net: backend.NetProxy}.Features()...); err != nil {
			return err
		}
	}
	if err := st.Update(ctx, func(st *state.VM) error { return netpolicy.SetOn(st, on) }); err != nil {
		return err
	}
	switch {
	case !running:
		e.Info(msg.NetSetStopped(to, vm))
	case e.restartNow(vm):
		if on {
			e.Note(msg.NetOnOldTokens(vm))
		}
		return Launch(e).Restart(ctx, Options{})
	default:
		e.Info(msg.NetSetRunning(to, vm))
	}
	if on {
		e.Note(msg.NetOnOldTokens(vm))
	}
	return nil
}

// LogOptions are what devvm network log shows of the network log.
type LogOptions struct {
	Lines   int  // the last events shown
	Follow  bool // then each new one, until ctx ends
	Blocked bool // the blocks grouped by host instead
}

// Log writes the folder's VM's network log to w: what its proxy allowed
// and blocked, and its links' connections. A VM without network policy
// logs only its links' connections, and devvm says so.
func (e Net) Log(ctx context.Context, w io.Writer, o LogOptions) error {
	l, vm, err := e.netLog(ctx, o.Follow)
	if err != nil || l == nil {
		if err == nil {
			e.Info(msg.LogEmpty(vm))
		}
		return err
	}
	if o.Blocked {
		return l.WriteBlocked(w)
	}
	return l.Write(ctx, w, o.Lines, o.Follow)
}

// LogEvents returns the last n events of the folder's VM's network log,
// oldest first: none when it has none.
func (e Net) LogEvents(ctx context.Context, n int) ([]netpolicy.Event, error) {
	l, _, err := e.netLog(ctx, false)
	if err != nil || l == nil {
		return nil, err
	}
	return l.Events(n)
}

// LogBlocks returns the blocks in the folder's VM's network log, one per
// host and port, by when each was last blocked: none when it has none.
func (e Net) LogBlocks(ctx context.Context) ([]netpolicy.Block, error) {
	l, _, err := e.netLog(ctx, false)
	if err != nil || l == nil {
		return nil, err
	}
	return l.Blocks()
}

// netLog returns the folder's VM's network log and the VM's name; a nil
// log when there's nothing to show (follow: nothing to follow). A VM
// without network policy logs only its links' connections, and devvm says
// so.
func (e Net) netLog(ctx context.Context, follow bool) (*netpolicy.Log, string, error) {
	t, err := finder(e).target(ctx, need{has: hasVM})
	if err != nil {
		return nil, "", err
	}
	vm := t.f.Name
	if !netpolicy.On(t.st) {
		e.Note(msg.NetOffLog(vm))
	}
	l := netpolicy.NewLog(e.Dirs, vm)
	// Its proxy makes the log as it starts: following an empty one waits.
	// One it just rotated is empty, with its events in the old one.
	fi, err := os.Stat(l.Path)
	_, oerr := os.Stat(l.Old())
	if errors.Is(err, os.ErrNotExist) || err == nil && fi.Size() == 0 && !follow && errors.Is(oerr, os.ErrNotExist) {
		return nil, vm, nil
	}
	return l, vm, nil
}

// widensPolicy returns the command, when a change to a VM's own rules gives
// it more access: adding allows, or removing a deny (a rule with its !);
// else "".
func widensPolicy(rules []string, deny, remove bool) string {
	switch {
	case !remove && !deny:
		return "devvm network allow"
	case remove && slices.ContainsFunc(rules, func(r string) bool { return strings.HasPrefix(strings.TrimSpace(r), "!") }):
		return "devvm network rm"
	}
	return ""
}

// PolicyChange adds rules to the folder's VM's own policy file (deny: as
// denies), or with remove takes them out; with global, the user's file
// for every VM instead. Its proxy reads the file again at its next
// connection, so a change applies at once.
func (e Net) PolicyChange(ctx context.Context, rules []string, deny, remove, global bool) error {
	file, vm := e.Dirs.Policy(), ""
	if !global {
		// The folder's VM, which its backend has, made for this folder.
		t, err := finder(e).target(ctx, need{has: hasMachine, folder: true, noVM: msg.NoVMPolicy})
		if err != nil {
			return err
		}
		if !netpolicy.On(t.st) {
			e.Note(msg.NetOffPolicy(t.f.Name))
		}
		if cmd := widensPolicy(rules, deny, remove); cmd != "" {
			if ok, err := t.widen(e.Env, cmd); !ok {
				return err
			}
		}
		vm = t.f.Name
		file = e.Dirs.VM(vm).Policy()
	}
	// The file's lines that aren't rules, once it's changed (or not).
	defer func() {
		for _, m := range netpolicy.PolicyWarnings([]string{file}) {
			e.Warn(m)
		}
	}()
	if remove {
		for _, r := range rules {
			switch ok, err := netpolicy.RemoveRule(file, r); {
			case err != nil:
				return err
			case !ok:
				e.Warn(msg.RuleMissing(file, r))
			case global:
				e.Info(msg.RuleRemovedGlobal(r))
			default:
				e.Info(msg.RuleRemoved(r, vm))
			}
		}
		return nil
	}
	// Every rule is checked before any is added.
	checked := make([]string, len(rules))
	for i, r := range rules {
		var err error
		if checked[i], err = netpolicy.Rule(r, deny); err != nil {
			return err
		}
	}
	for _, r := range checked {
		switch ok, err := netpolicy.AddRule(file, r); {
		case err != nil:
			return err
		case !ok:
			e.Info(msg.RulePresent(file, r))
		case global:
			e.Info(msg.RuleAddedGlobal(r))
		default:
			e.Info(msg.RuleAdded(r, vm))
		}
	}
	return nil
}
