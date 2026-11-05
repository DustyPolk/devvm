// Package sshconfig is how ssh and editors reach project VMs by name: the
// ssh config devvm writes for its running VMs (devvm-<folder>), each
// checked against the host keys devvm pins for its start
// (vmssh.PinHostKeys), and the line in ~/.ssh/config that includes it.
package sshconfig

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmname"
	"github.com/w3cj/devvm/internal/vmssh"
)

// header starts the ssh config devvm writes.
const header = "# Written by devvm on every launch; edits are overwritten.\n"

// Config is devvm's ssh config. New sets its
// backends; its caller sets what it prints and asks.
type Config struct {
	Dirs paths.Dirs
	// Backends returns the backends asked for VMs: those some VM's state
	// names, read as each config is written.
	Backends func() ([]backend.Backend, error)
	Review   backend.Review // what their ssh is checked against
	// What it prints, and asks: [Y/n]; Terminal reports whether it can
	// ask.
	Info, Warn, Say func(msg.M)
	Confirm         func(msg.M) bool
	Terminal        func() bool
	// AddInclude adds the Include line without asking (--ssh-config).
	AddInclude bool
}

// New returns the ssh config for the VMs of the backends in reg that some
// VM's state names.
func New(d paths.Dirs, reg *backend.Registry) *Config {
	return &Config{Dirs: d, Backends: func() ([]backend.Backend, error) { return reg.Used(d) }, Review: reg.Review}
}

// ChooseAlias is the name ssh and editors will reach a new VM, vm, by
// besides its own: devvm-<folder>, as its name has the folder's name,
// unless another VM's state records that alias; then the VM's own name.
// It's chosen once, as the VM is made (state.VM.Alias), so a VM's alias
// never changes when others come and go, inside the claim of the VM's
// state.VM.Create, so two VMs made at once never choose one alias.
func ChooseAlias(d paths.Dirs, vm string) (string, error) {
	short, ok := vmname.Short(vm)
	if !ok {
		return vm, nil
	}
	names, err := state.VMNames(d)
	if err != nil {
		return "", err
	}
	for _, n := range names {
		if n == vm {
			continue
		}
		st, err := state.ReadVM(d, n)
		if err != nil {
			return "", err
		}
		if st.Alias == short && vmname.IsAlias(st.Alias, n) {
			return vm, nil
		}
	}
	return short, nil
}

// Host is the name ssh and editors reach vm by: the alias its state
// records, else its own name. An alias ChooseAlias never gives vm (a
// hand-edited vm.json's: another folder's, a pattern, more lines for the
// ssh config) is never taken (vmname.IsAlias).
func (c *Config) Host(vm string) string {
	if st, err := state.ReadVM(c.Dirs, vm); err == nil && vmname.IsAlias(st.Alias, vm) {
		return st.Alias
	}
	return vm
}

type running struct {
	b  backend.Backend
	in backend.Instance
}

// running returns the running project VMs of the backends some VM's
// state names. A backend that can't list runs nothing to reach.
func (c *Config) running(ctx context.Context) ([]running, error) {
	bs, err := c.Backends()
	if err != nil {
		return nil, err
	}
	var up []running
	for _, b := range bs {
		ins, err := b.List(ctx)
		if err != nil {
			continue
		}
		for _, in := range ins {
			if vmname.IsVM(in.Name) && in.Status == backend.StatusRunning {
				up = append(up, running{b, in})
			}
		}
	}
	return up, nil
}

// Write rewrites the ssh config with every running project VM, each by
// its alias (Host) and its full name, neither of which ever changes,
// checked against the host keys devvm pins for its current start
// (vmssh.PinHostKeys), read from it once. A VM whose host keys can't be
// pinned gets no entry: a stopped VM's port is free for anything on the host to
// take, so ssh must check it reaches the VM. Nor does one whose backend's
// ssh isn't usable (an emulation not yet reviewed), or isn't a user and a
// key on a host loopback port (vmssh.FromTarget). An alias an earlier
// entry has (two VMs made at once) is left out: ssh would take the first.
func (c *Config) Write(ctx context.Context) error {
	up, err := c.running(ctx)
	if err != nil {
		return err
	}
	slices.SortFunc(up, func(a, b running) int { return strings.Compare(a.in.Name, b.in.Name) })
	var buf strings.Builder
	buf.WriteString(header)
	taken := map[string]bool{}
	for _, r := range up {
		if !c.Review.Allowed(r.b, backend.FeatureSSH) {
			continue
		}
		t, err := r.b.SSH(ctx, r.in.Name)
		if err != nil {
			continue
		}
		login, err := vmssh.FromTarget(r.in.Name, t)
		if err != nil {
			continue
		}
		run, err := r.b.Running(ctx, r.in.Name)
		if err != nil {
			continue
		}
		kh, err := vmssh.PinHostKeys(ctx, c.Dirs, r.b, r.in.Name)
		if errors.Is(err, vmssh.ErrNoHostKeys) {
			continue
		} else if err != nil {
			return err
		}
		login.KnownHosts = kh
		alias := c.Host(r.in.Name)
		if taken[alias] {
			alias = r.in.Name
		}
		taken[alias] = true
		if err := c.entry(&buf, alias, r.in, login, t.Options, run.ID); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return state.WriteFile(c.Dirs.SSHConfig(), []byte(buf.String()))
}

// entry writes vm's Host block. Its own host keys are checked, and agent
// forwarding is the choice its current start (start) was made with, as
// devvm's own shell has it (sshagent.Running), an explicit no included, so
// a Host * of the user's doesn't apply: ssh takes an option's first value.
// It logs in as login says, its backend's checked target (vmssh.FromTarget),
// with the target's other login options (opts).
func (c *Config) entry(buf *strings.Builder, alias string, in backend.Instance, login vmssh.SSH, opts []backend.SSHOption, start string) error {
	hosts := alias
	if alias != in.Name {
		hosts += " " + in.Name
	}
	st, err := state.ReadVM(c.Dirs, in.Name)
	if err != nil {
		return err
	}
	forward, identity := "no", ""
	switch sshagent.Running(st, start) {
	case sshagent.On:
		forward = "yes"
	case sshagent.Devvm:
		// Editors forward devvm's own agent too. OpenSSH forwards a
		// socket only when it sees an agent of its own: the same one,
		// so an editor started without SSH_AUTH_SOCK (or with another)
		// still forwards it.
		forward = `"` + c.Dirs.AgentSock() + `"`
		identity = "  IdentityAgent " + forward
	}
	lines := []string{
		"", "Host " + hosts,
		"  HostKeyAlias " + in.Name,
		`  UserKnownHostsFile "` + login.KnownHosts + `"`,
		"  StrictHostKeyChecking yes",
		"  NoHostAuthenticationForLocalhost no",
		"  ForwardAgent " + forward,
		// A connection of its own: over a shared one, a session would
		// forward the agent of whichever process opened it.
		"  ControlMaster no",
		"  ControlPath none",
	}
	if identity != "" {
		lines = append(lines, identity)
	}
	lines = append(lines,
		// Nothing else of the host's for the VM, whatever a Host * says:
		// no X11 display, tunnel or remote SOCKS forward, none of its
		// keys added to an agent, no hop through another host or a
		// command, and no host keys the VM offers later (devvm reads them
		// itself). A RemoteForward can't be turned off here without
		// refusing -R on the command line too. PermitRemoteOpen needs
		// OpenSSH 8.7 or newer. No IgnoreUnknown: its first value wins,
		// and would replace a Host * one of the user's.
		"  ForwardX11 no",
		"  ForwardX11Trusted no",
		"  Tunnel no",
		"  PermitRemoteOpen none",
		"  AddKeysToAgent no",
		"  ProxyJump none",
		"  ProxyCommand none",
		"  UpdateHostKeys no",
		"  HashKnownHosts no",
	)
	lines = append(lines,
		"  Hostname "+vmssh.Loopback, "  Port "+strconv.Itoa(login.Port),
		"  User "+login.User, `  IdentityFile "`+login.Key+`"`,
	)
	for _, o := range opts {
		lines = append(lines, "  "+o.Key+" "+o.Value)
	}
	buf.WriteString(strings.Join(lines, "\n") + "\n")
	return nil
}

// IncludeLine is the line ~/.ssh/config needs to read devvm's ssh config.
func (c *Config) IncludeLine() string { return "Include " + c.Dirs.SSHConfig() }

// EnsureInclude makes sure home's ~/.ssh/config includes devvm's ssh
// config, asking first: once said yes to, the line goes at the top, where
// it must be, before any Host block. It reports whether the line is
// there. Without a terminal it says to add it.
func (c *Config) EnsureInclude(home string) (bool, error) {
	line := c.IncludeLine()
	sshDir := filepath.Join(home, ".ssh")
	path := filepath.Join(sshDir, "config")
	data, err := os.ReadFile(path) //nolint:gosec // the user's ssh config
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if bytes.Contains(data, []byte(line)) {
		return true, nil
	}
	c.Say(msg.SSHIncludeNeeded(line))
	switch {
	case c.AddInclude:
	case !c.Terminal():
		c.Warn(msg.SSHIncludeYourself())
		return false, nil
	case !c.Confirm(msg.AddItNow()):
		return false, nil
	}
	// The user's ~/.ssh, which may be a link (a dotfiles repo's): followed,
	// as state.PrivateDir, for devvm's own folders, wouldn't.
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return false, err
	}
	if err := os.Chmod(sshDir, 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
		return false, err
	}
	// A linked config (a dotfiles repo) stays linked: its file is written.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	if err := state.WriteFile(path, append([]byte(line+"\n"), data...)); err != nil {
		return false, err
	}
	c.Info(msg.SSHIncludeAdded())
	return true, nil
}
