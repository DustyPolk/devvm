package vm

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// Net shows and changes the folder's VM's network policy, its network log
// and its allowlist (here), and the secrets its proxy puts in
// (secret.go).
type Net struct{ *Env }

// Network writes the folder's VM's network policy to w, as devvm ls
// shows it: on, off or unenforced.
func (e Net) Network(ctx context.Context, w io.Writer) error {
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	in, err := t.b.Inspect(ctx, t.f.Name)
	if err != nil {
		return err
	}
	shown, err := netpolicy.Listed(ctx, t.b, t.st, in.Status == backend.StatusRunning)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, shown+"\n")
	return err
}

// SetNetwork turns the folder's VM's network policy on or off, from its
// next start. Turning it on, its backend must be able to confine the VM
// to its proxy, and tokens the VM got before stay valid.
func (e Net) SetNetwork(ctx context.Context, on bool) error {
	t, err := e.existing(ctx, "")
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	to := "off"
	if on {
		to = "on"
	}
	if st.Get(KeyNet) == to {
		e.Info(msg.NetAlready(to, vm))
		return nil
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
	if err := st.Set(KeyNet, to); err != nil {
		return err
	}
	if running {
		e.Info(msg.NetSetRunning(to, vm))
	} else {
		e.Info(msg.NetSetStopped(to, vm))
	}
	if on {
		e.Note(msg.NetOnOldTokens(vm))
	}
	return nil
}

// LogOptions are what devvm log shows of the network log.
type LogOptions struct {
	Lines   int  // the last events shown
	Follow  bool // then each new one, until ctx ends
	Blocked bool // the blocks grouped by host instead
}

// Log writes the folder's VM's network log to w: what its proxy allowed
// and blocked. A VM without network policy logs nothing new, and devvm
// says so.
func (e Net) Log(ctx context.Context, w io.Writer, o LogOptions) error {
	t, err := e.findExisting(ctx, "")
	if err != nil {
		return err
	}
	vm := t.f.Name
	if !netpolicy.On(t.st) {
		e.Note(msg.NetOffLog(vm))
	}
	l := netpolicy.NewLog(e.Dirs, vm)
	// Its proxy makes the log as it starts: following an empty one waits.
	if fi, err := os.Stat(l.Path); errors.Is(err, os.ErrNotExist) || err == nil && fi.Size() == 0 && !o.Follow {
		e.Info(msg.LogEmpty(vm))
		return nil
	}
	if o.Blocked {
		return l.WriteBlocked(w)
	}
	return l.Write(ctx, w, o.Lines, o.Follow)
}

// policyVM finds the folder's VM for devvm policy, which must exist and
// be made for this folder; noVM is the error when it doesn't exist.
func (e Net) policyVM(ctx context.Context, noVM func(dir string) msg.M) (*found, error) {
	t, err := e.find(ctx, "")
	if err != nil {
		return nil, err
	}
	if !t.exists {
		return nil, noVM(t.f.Dir)
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, err
	}
	if !netpolicy.On(t.st) {
		e.Note(msg.NetOffPolicy(t.f.Name))
	}
	return t, nil
}

// Policy writes the folder's VM's allowlist to w: each policy file its
// proxy reads that has rules, in order.
func (e Net) Policy(ctx context.Context, w io.Writer) error {
	t, err := e.policyVM(ctx, msg.NoVM)
	if err != nil {
		return err
	}
	files, err := e.Proxy.PolicyFiles(t.f.Name, t.st.Get(state.KeyImage))
	if err != nil {
		return err
	}
	return netpolicy.WriteRules(w, files)
}

// PolicyChange adds rules to the folder's VM's own policy file (deny: as
// denies), or with remove takes them out; with global, the user's file
// for every VM instead. Its proxy reads the file again at its next
// connection, so a change applies at once.
func (e Net) PolicyChange(ctx context.Context, rules []string, deny, remove, global bool) error {
	file, vm := e.Dirs.Policy(), ""
	if !global {
		t, err := e.policyVM(ctx, msg.NoVMPolicy)
		if err != nil {
			return err
		}
		vm = t.f.Name
		file = netpolicy.VMPolicyFile(e.Dirs, vm)
	}
	if remove {
		for _, r := range rules {
			switch ok, err := netpolicy.RemoveRule(file, r); {
			case err != nil:
				return err
			case !ok:
				e.Warn(msg.RuleMissing(file, msg.Shown(r)))
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
