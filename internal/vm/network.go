package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/image"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/netpolicy"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/settings"
	"github.com/devvmhq/devvm/internal/state"
	"github.com/devvmhq/devvm/internal/tools"
)

// Net shows and changes the folder's VM's network tier, its network log
// and its allowlist (here), and the secrets its proxy puts in
// (secret.go).
type Net struct{ *Env }

// NetworkShown is the folder's VM's network tier, as devvm network
// shows it.
type NetworkShown struct {
	Policy string // its tier or unenforced, as devvm ls shows it
	// Rules are the rules of each policy file its proxy reads that has
	// any, in order.
	Rules []netpolicy.RuleFile
}

// Network returns the folder's VM's network tier and its allowlist.
func (e Net) Network(ctx context.Context) (NetworkShown, error) {
	var s NetworkShown
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return s, err
	}
	if s.Policy, err = netpolicy.Listed(ctx, t.b, t.st, t.in.Status == backend.StatusRunning); err != nil {
		return s, err
	}
	set := netpolicy.Policies(e.Dirs, t.f.Name, netpolicy.TierOf(t.st), image.Name(t.st.Image), tools.Names(t.st))
	s.Rules, err = netpolicy.ReadPolicies(e.Proxy.Assets, set)
	return s, err
}

// NetworkDefault returns new VMs' network tier, or ask, and where that
// came from.
func (e Net) NetworkDefault() (settings.Value, error) {
	def, err := settings.Read(e.Dirs)
	return def.Network, err
}

// SetNetworkDefault saves new VMs' network tier, or ask, as devvm setup
// does (NetworkChoice).
func (e Net) SetNetworkDefault(mode string) error {
	return NetworkChoice{Dirs: e.Dirs, Reporter: e.Reporter}.Set(mode)
}

// NetworkChoice chooses new VMs' network tier, or ask, which devvm
// network -g and devvm setup save in settings.json. devvm's --network
// still chooses for one VM.
type NetworkChoice struct {
	Dirs         paths.Dirs
	msg.Reporter // says what's saved
}

// Set saves mode (a settings.NetworkChoices value) as new VMs' network
// tier; config.toml mustn't set it (set_in_config).
func (c NetworkChoice) Set(mode string) error {
	if _, err := settings.Save(c.Dirs, settings.Values{Network: &mode}); err != nil {
		return err
	}
	c.Info(msg.NetworkDefault(mode))
	return nil
}

// SetTier gives the folder's VM the tier to, as command (devvm network
// allowlist) asks. Between confined tiers it applies at once: a running VM's
// proxy starts again with the new tier's rules. Into or out of open it
// applies from the VM's next start: a running VM restarts now once the
// user agrees (restartNow). A confined tier needs a backend that can
// confine the VM to its proxy, and tokens the VM got while it was open
// stay valid. A lower tier needs the widening check (widen). Any change
// is refused while it would break links' tier rule (tierLinked), and
// while the VM is in a group, whose VMs are all on one tier.
func (e Net) SetTier(ctx context.Context, to netpolicy.Tier, command string) error {
	t, err := e.finder.target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	from := netpolicy.TierOf(st)
	if from == to {
		e.Info(msg.SettingAlready(command, vm))
		return nil
	}
	if g := groupOf(st); g != "" {
		return msg.GroupTierFixed(g, vm)
	}
	if err := tierLinked(e.Dirs, st, to); err != nil {
		return err
	}
	if !to.AtLeast(from) {
		if ok, err := t.widen(command); !ok {
			return err
		}
	}
	if to.Confined() {
		if err := e.Registry.Require(b, vm, backend.Confinement{Net: to.Net()}.Features()...); err != nil {
			return err
		}
	}
	if err := st.Update(ctx, func(st *state.VM) error { return netpolicy.SetTier(st, to) }); err != nil {
		return err
	}
	running := t.in.Status == backend.StatusRunning
	if from.Confined() && to.Confined() {
		if running {
			if _, err := e.Proxy.Prepare(ctx, b, st, true); err != nil {
				return err
			}
			e.Info(msg.SettingApplied(vm))
			return nil
		}
		e.Info(msg.SettingSet(vm))
		return nil
	}
	switch {
	case !running:
		e.Info(msg.SettingSet(vm))
	case e.restartNow(vm, e.Confirm, 1):
		return e.launcher.Restart(ctx, Options{})
	default:
		e.Info(msg.NextStart(1, vm))
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
// and blocked, and its links' connections. An open VM
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
// log when there's nothing to show (follow: nothing to follow). An open
// VM's log has only what goes through its proxy, and devvm says so.
func (e Net) netLog(ctx context.Context, follow bool) (*netpolicy.Log, string, error) {
	t, err := e.finder.target(ctx, need{has: hasVM})
	if err != nil {
		return nil, "", err
	}
	vm := t.f.Name
	if !netpolicy.TierOf(t.st).Confined() {
		e.Note(msg.NetOpenLog(vm))
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
		t, err := e.finder.target(ctx, need{has: hasMachine, folder: true, noVM: msg.NoVMPolicy})
		if err != nil {
			return err
		}
		switch tier := netpolicy.TierOf(t.st); {
		case !tier.Confined():
			e.Note(msg.NetOpenPolicy(t.f.Name))
		case tier != netpolicy.TierAllowlist && !deny && !remove:
			e.Note(msg.NetAllowsUnused(string(tier), t.f.Name))
		}
		if cmd := widensPolicy(rules, deny, remove); cmd != "" {
			if ok, err := t.widen(cmd); !ok {
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
