// Package sshconfig is how ssh and editors reach project VMs by name: the
// ssh config devvm writes for its running VMs (devvm-<folder>), the VMs'
// host keys it checks them with, and the line in ~/.ssh/config that
// includes it.
package sshconfig

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmname"
)

// header starts the ssh config devvm writes.
const header = "# Written by devvm on every launch; edits are overwritten.\n"

// hostKeyTimeout is how long a VM gets to show its host keys.
const hostKeyTimeout = 30 * time.Second

// hostKeysMax is the most of a VM's answer read for its host keys.
const hostKeysMax = 64 << 10

// Config is devvm's ssh config and its VMs' host keys. New sets its
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
// It's chosen once, as the VM is made (state.KeyAlias), so a VM's alias
// never changes when others come and go.
func ChooseAlias(d paths.Dirs, vm string) (string, error) {
	slug, _ := vmname.SlugOf(vm)
	short := vmname.Prefix + slug
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
		if st.Get(state.KeyAlias) == short {
			return vm, nil
		}
	}
	return short, nil
}

// Host is the name ssh and editors reach vm by: the alias its state
// records, else its own name.
func (c *Config) Host(vm string) string {
	if st, err := state.ReadVM(c.Dirs, vm); err == nil && st.Get(state.KeyAlias) != "" {
		return st.Get(state.KeyAlias)
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
// checked against its own host keys. A VM whose host keys can't be read
// gets no entry: a stopped VM's port is free for anything on the host to
// take, so ssh must check it reaches the VM. Nor does one whose backend's
// ssh isn't usable (an emulation not yet reviewed). An alias an earlier
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
		if err != nil || t.Host == "" || t.Port == 0 && t.ProxyCommand == "" || strings.ContainsAny(t.ProxyCommand, "\r\n") {
			continue
		}
		start, err := r.b.StartID(ctx, r.in.Name)
		if err != nil {
			continue
		}
		ok, err := c.recordHostKeys(ctx, r.b, r.in.Name)
		if err != nil {
			return err
		}
		if ok {
			alias := c.Host(r.in.Name)
			if taken[alias] {
				alias = r.in.Name
			}
			taken[alias] = true
			if err := c.entry(&buf, alias, r.in, t, start); err != nil {
				return err
			}
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
func (c *Config) entry(buf *strings.Builder, alias string, in backend.Instance, t backend.SSHTarget, start string) error {
	hosts := alias
	if alias != in.Name {
		hosts += " " + in.Name
	}
	st, err := state.ReadVM(c.Dirs, in.Name)
	if err != nil {
		return err
	}
	// A backend's own way in (Docker's docker exec), else none, whatever a
	// Host * of the user's says.
	proxy := "none"
	if t.ProxyCommand != "" {
		proxy = t.ProxyCommand
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
		`  UserKnownHostsFile "` + c.Dirs.KnownHosts() + `"`,
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
		// keys added to an agent, no hop through another host, and no
		// host keys the VM offers later (devvm reads them itself). A
		// RemoteForward can't be turned off here without refusing -R on
		// the command line too. PermitRemoteOpen needs OpenSSH 8.7 or
		// newer. No IgnoreUnknown: its first value wins, and would replace
		// a Host * one of the user's.
		"  ForwardX11 no",
		"  ForwardX11Trusted no",
		"  Tunnel no",
		"  PermitRemoteOpen none",
		"  AddKeysToAgent no",
		"  ProxyJump none",
		"  ProxyCommand "+proxy,
		"  UpdateHostKeys no",
		"  HashKnownHosts no",
	)
	lines = append(lines, "  Hostname "+t.Host)
	if t.Port > 0 {
		lines = append(lines, "  Port "+strconv.Itoa(t.Port))
	}
	if t.User != "" {
		lines = append(lines, "  User "+t.User)
	}
	for _, o := range t.Options {
		lines = append(lines, "  "+o.Key+" "+o.Value)
	}
	buf.WriteString(strings.Join(lines, "\n") + "\n")
	return nil
}

// A host key as a VM shows it: its type and its key.
var (
	keyType = regexp.MustCompile(`^(ssh|ecdsa)-[A-Za-z0-9@.-]+$`)
	keyData = regexp.MustCompile(`^[A-Za-z0-9+/]+=*$`)
)

// recordHostKeys makes sure the known hosts file has vm's host keys,
// reading them from the running VM through its backend (which reaches
// the VM itself) when it has none. It reports whether it has them.
func (c *Config) recordHostKeys(ctx context.Context, b backend.Backend, vm string) (bool, error) {
	path := c.Dirs.KnownHosts()
	data, err := os.ReadFile(path) //nolint:gosec // devvm's own file
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if hasHost(data, vm) {
		return true, nil
	}
	kctx, cancel := context.WithTimeout(ctx, hostKeyTimeout)
	defer cancel()
	out := &capped{max: hostKeysMax}
	err = b.Exec(kctx, vm, []string{"sh", "-c", "cat /etc/ssh/ssh_host_*_key.pub"}, backend.ExecOpts{Stdout: out})
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err != nil {
		return false, nil
	}
	keys := parseHostKeys(vm, out.Bytes())
	if keys == "" {
		return false, nil
	}
	return true, state.WriteFile(path, append(data, keys...))
}

// HostKey returns vm's host key as the known hosts file pins it (TYPE
// BASE64), its ed25519 one where it has one, reading them from the
// running VM first when it has none.
func (c *Config) HostKey(ctx context.Context, b backend.Backend, vm string) (string, error) {
	ok, err := c.recordHostKeys(ctx, b, vm)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New(vm + ": its host keys can't be read")
	}
	data, err := os.ReadFile(c.Dirs.KnownHosts())
	if err != nil {
		return "", err
	}
	key := ""
	for l := range strings.SplitSeq(string(data), "\n") {
		k, ok := strings.CutPrefix(l, vm+" ")
		if !ok {
			continue
		}
		if key == "" || strings.HasPrefix(k, "ssh-ed25519 ") {
			key = k
		}
	}
	if key == "" {
		return "", errors.New(vm + ": no host key pinned")
	}
	return key, nil
}

// parseHostKeys returns the known hosts lines for the host keys in out,
// under vm's name; lines that aren't a host key are dropped.
func parseHostKeys(vm string, out []byte) string {
	var b strings.Builder
	s := bufio.NewScanner(bytes.NewReader(out))
	for s.Scan() {
		f := strings.Fields(s.Text())
		if len(f) >= 2 && keyType.MatchString(f[0]) && keyData.MatchString(f[1]) {
			b.WriteString(vm + " " + f[0] + " " + f[1] + "\n")
		}
	}
	return b.String()
}

// hasHost reports whether known hosts data has a line for vm.
func hasHost(data []byte, vm string) bool {
	for l := range strings.SplitSeq(string(data), "\n") {
		if strings.HasPrefix(l, vm+" ") {
			return true
		}
	}
	return false
}

// ForgetHostKeys drops vm's host keys, to be read again once it runs: a
// VM whose backend config changed can get new ones when it boots.
func (c *Config) ForgetHostKeys(vm string) error {
	path := c.Dirs.KnownHosts()
	data, err := os.ReadFile(path) //nolint:gosec // devvm's own file
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !hasHost(data, vm) {
		return err
	}
	var keep []string
	for l := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		if !strings.HasPrefix(l, vm+" ") {
			keep = append(keep, l)
		}
	}
	out := ""
	if len(keep) > 0 {
		out = strings.Join(keep, "\n") + "\n"
	}
	return state.WriteFile(path, []byte(out))
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

// capped keeps the first max bytes written to it, and takes the rest
// without keeping it.
type capped struct {
	bytes.Buffer
	max int
}

func (w *capped) Write(p []byte) (int, error) {
	if room := w.max - w.Len(); room > 0 {
		w.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
