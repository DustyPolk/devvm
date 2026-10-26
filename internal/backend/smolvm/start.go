package smolvm

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// A machine with network policy (NetProxy) reaches nothing but devvm's
// proxy, in three parts:
//
//   - its allow list is the gateway alone (gatewayCIDR), so smolvm refuses
//     connections anywhere else, and the gateway is every port of the host's
//     loopback;
//   - its DNS answers only dnsClosed, a name that doesn't exist (smolvm
//     answers every other query itself, NXDOMAIN, without asking anyone);
//   - it starts with mediated egress (--mediated-egress --egress-interceptor):
//     smolvm asks devvm's proxy, as its decider, about each connection the
//     allow list lets through, before it connects anywhere. The proxy takes
//     over those to itself and refuses the rest. A decider that's gone, or
//     doesn't know the token, refuses everything; and once a machine started
//     so, smolvm won't start it without a decider.
const (
	gatewayCIDR = Gateway + "/32"
	dnsClosed   = "devvm.invalid"
)

// Start starts a stopped machine with the network c asks for, then what
// base's services do at boot on Lima (services.sh): smolvm runs no init
// system. A running machine is left as it is.
//
// Its network and environment are set while it's stopped, where they
// differ: NetDirect and NetInternet are smolvm's own network, which reaches
// the internet but not the host's loopback; NetNone is none; NetProxy is
// devvm's proxy alone, through its decider (c.Decider), with the proxy's
// address in HTTP_PROXY and HTTPS_PROXY (and lowercase), which devvm's
// execs get from the machine's proxy file (proxyFile), so the password is
// in no process's arguments. A machine made forwarding an SSH agent
// forwards the one agentSocket chooses: none, c.AgentSocket, or the
// user's (SSH_AUTH_SOCK). With c.SSH its ssh is forwarded (sshForward) and
// services.sh runs its sshd; without, the forward goes. A VM that
// forwards every port gets its forwarder once its sshd runs (forward).
func (s *Smolvm) Start(ctx context.Context, vm string, c backend.Confinement) error {
	proxy := c.Net == backend.NetProxy
	if proxy {
		if err := checkDecider(vm, c); err != nil {
			return err
		}
	}
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return notFound(vm, err)
	}
	if m.status() == backend.StatusRunning {
		return nil
	}
	r, err := s.readRecord(vm)
	if err != nil {
		return err
	}
	agent := ""
	if r.Forwarding {
		if agent, err = agentSocket(vm, r, c, s.Getenv); err != nil {
			return err
		}
	}
	var opts []string
	if proxy {
		if !m.closed() {
			opts = append(opts, "--reset-allow-list", "--allow-cidr", gatewayCIDR, "--allow-host-pattern", dnsClosed)
		}
	} else {
		// What a start through the decider left: the allow list, and the
		// decider required at every start.
		if len(m.AllowedCIDRs) > 0 || len(m.AllowedHosts) > 0 || r.Mediated {
			if err := s.update(ctx, vm, "--net", "--reset-allow-list", "--no-egress-interceptor"); err != nil {
				return err
			}
			m.Network, r.Mediated = true, false
		}
		if want := c.Net != backend.NetNone; m.Network != want {
			if want {
				opts = append(opts, "--net")
			} else {
				opts = append(opts, "--no-net")
			}
		}
	}
	env := maps.Clone(c.Env)
	for _, k := range proxyVars {
		delete(env, k) // the proxy file's
	}
	if opts, err = listedPorts(vm, c, &r, opts); err != nil {
		return err
	}
	// The VM's ssh: forwarded while it's asked for, its sshd run then.
	var sshd []string
	if c.SSH {
		port, err := s.sshForward(ctx, vm, c, r)
		if err != nil {
			return err
		}
		pub, err := s.ensureSSHKey(vm)
		if err != nil {
			return err
		}
		name, _, err := s.user()
		if err != nil {
			return err
		}
		if port != r.SSHPort {
			if r.SSHPort > 0 {
				opts = append(opts, "--remove-port", sshForwardSpec(r.SSHPort))
			}
			opts = append(opts, "-p", sshForwardSpec(port))
		}
		r.SSHPort, sshd = port, servicesArgs(name, pub, env, proxy)
	} else if r.SSHPort > 0 {
		opts = append(opts, "--remove-port", sshForwardSpec(r.SSHPort))
		r.SSHPort = 0
	}
	if err := s.update(ctx, vm, opts...); err != nil {
		return err
	}
	if !maps.Equal(r.Env, env) && (len(r.Env) > 0 || len(env) > 0) {
		var eopts []string
		for _, k := range slices.Sorted(maps.Keys(r.Env)) {
			if _, ok := env[k]; !ok {
				eopts = append(eopts, "--remove-env", k)
			}
		}
		for _, kv := range sortedEnv(env) {
			eopts = append(eopts, "-e", kv)
		}
		if err := s.update(ctx, vm, eopts...); err != nil {
			return err
		}
		r.Env = env
	}
	if err := s.setProxyFile(vm, s.ProxyURL(c)); err != nil {
		return err
	}
	// Recorded before the start: a start that fails leaves it unconfined,
	// and without ssh until services.sh ran its sshd.
	r.Mediated, r.Confined, r.Started = r.Mediated || proxy, "", ""
	r.SSHUp, r.NoSSHD = false, false
	if err := s.writeRecord(vm, r); err != nil {
		return err
	}
	args := []string{"machine", "start", "--name", vm}
	if proxy {
		args = append(args, "--mediated-egress", "--egress-interceptor", c.Decider)
	}
	cmd := s.smolvm(ctx, args...)
	// The token is in no process's arguments.
	if proxy {
		cmd.Env = setEnv(cmd.Env, "SMOLVM_INTERCEPTOR_TOKEN="+c.DeciderToken)
	} else {
		cmd.Env = setEnv(cmd.Env, "SMOLVM_INTERCEPTOR_TOKEN")
	}
	// The agent it forwards (agentSocket), read at this start alone; one
	// made without forwarding gets none (smolvm's commands never pass on
	// devvm's own SSH_AUTH_SOCK).
	if agent != "" {
		cmd.Env = setEnv(cmd.Env, "SSH_AUTH_SOCK="+agent)
	}
	if err := s.Runner.Run(ctx, msg.Starting(vm), cmd); err != nil {
		return backend.Logged(msg.StartFailed(vm))
	}
	if r.Started, err = s.lifeline(ctx, vm); err != nil {
		return err
	}
	if proxy {
		r.Confined = r.Started
	}
	if err := s.writeRecord(vm, r); err != nil {
		return err
	}
	noSSHD := false
	if sshd != nil {
		if noSSHD, err = s.lacksSSHD(ctx, vm); err != nil {
			return err
		}
		if noSSHD {
			s.Warn(msg.SmolvmNoSshd(vm))
			sshd = nil
		}
	}
	if err := s.services(ctx, vm, sshd); err != nil {
		return err
	}
	if r.SSHPort > 0 {
		r.SSHUp, r.NoSSHD = !noSSHD, noSSHD
		if err := s.writeRecord(vm, r); err != nil {
			return err
		}
	}
	if r.PortsAll {
		s.forward(ctx, vm, r, c)
	}
	return nil
}

// listedPorts returns opts with machine update's options that make a
// stopped machine forward the listed ports c has for this start (c.Ports:
// those a host program has left out), where they differ from what it
// forwards (r, updated); none without c.Ports.
func listedPorts(vm string, c backend.Confinement, r *record, opts []string) ([]string, error) {
	if c.Ports == nil {
		return opts, nil
	}
	want, err := portArgs(vm, *c.Ports)
	if err != nil || slices.Equal(want, r.Ports) {
		return opts, err
	}
	for _, old := range r.Ports {
		opts = append(opts, "--remove-port", old)
	}
	for _, w := range want {
		opts = append(opts, "-p", w)
	}
	r.Ports = want
	return opts, nil
}

// forward starts the forwarder of a VM that forwards every port, once its
// sshd runs (forward.go): its host key read through smolvm, and never on
// its own ssh's, proxy's or decider's port. One that can't start is
// warned about: the VM runs on.
func (s *Smolvm) forward(ctx context.Context, vm string, r record, c backend.Confinement) {
	if !r.SSHUp {
		s.Warn(msg.SmolvmPortsNoSSH(vm))
		return
	}
	failed := func() { s.Warn(msg.SmolvmPortsFailed(s.Forwards.Log(vm), vm)) }
	out, err := vmOutput(ctx, s.Runner, func(ctx context.Context) *exec.Cmd {
		return s.smolvm(ctx, "machine", "exec", "--name", vm, "--", "cat", "/etc/ssh/ssh_host_ed25519_key.pub")
	})
	key, ok := hostKey(out)
	name, _, uerr := s.user()
	if err != nil || !ok || uerr != nil {
		failed()
		return
	}
	spec := forwardSpec{
		lifeline: s.Lifeline(ctx, vm), user: name, key: s.sshKey(vm), hostKey: key, sshPort: r.SSHPort,
		reserve: []int{r.SSHPort, c.ProxyPort, deciderPort(c.Decider)},
	}
	if spec.lifeline == "" || s.Forwards.Start(vm, spec) != nil {
		failed()
	}
}

// What devvm reads of what the VM prints, and how long it waits for it:
// root in the VM decides both.
const (
	vmOutputMax     = 64 << 10
	vmOutputTimeout = 30 * time.Second
)

// vmOutput runs the command mk makes, whose output the VM makes, through
// r, returning at most vmOutputMax bytes of it and waiting vmOutputTimeout
// at most.
func vmOutput(ctx context.Context, r Runner, mk func(context.Context) *exec.Cmd) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, vmOutputTimeout)
	defer cancel()
	out := &capped{max: vmOutputMax}
	cmd := mk(ctx)
	cmd.Stdout, cmd.WaitDelay = out, time.Second
	err := r.Attach(cmd)
	return out.Bytes(), err
}

// capped keeps the first max bytes written to it.
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

// sshdProbe answers "sshd" when the machine's image has sshd, else "none".
const sshdProbe = `if [ -x /usr/sbin/sshd ] || command -v sshd >/dev/null 2>&1; then echo sshd; else echo none; fi`

// lacksSSHD reports whether a running machine's image has no sshd (made
// before devvm installed one), asked before services.sh runs, so no other
// failure reads as it; one that can't say stops it.
func (s *Smolvm) lacksSSHD(ctx context.Context, vm string) (bool, error) {
	out, err := vmOutput(ctx, s.Runner, func(ctx context.Context) *exec.Cmd {
		return s.smolvm(ctx, "machine", "exec", "--name", vm, "--", "sh", "-c", sshdProbe)
	})
	switch strings.TrimSpace(string(out)) {
	case "none":
		return true, nil
	case "sshd":
		if err == nil {
			return false, nil
		}
	}
	if ctx.Err() == nil {
		_ = s.stop(ctx, vm)
	}
	return false, backend.Logged(msg.StartFailed(vm))
}

// closed reports whether the machine's allow lists are network policy's:
// the gateway alone, and DNS for dnsClosed alone. smolvm's JSON doesn't show
// its UDP allows, deny lists or egress rules.
func (m machine) closed() bool {
	return slices.Equal(m.AllowedCIDRs, []string{gatewayCIDR}) && slices.Equal(m.AllowedHosts, []string{"=" + dnsClosed})
}

// checkDecider checks a NetProxy start has a decider: on host loopback,
// with a token of 64 hex digits.
func checkDecider(vm string, c backend.Confinement) error {
	at, err := netip.ParseAddrPort(c.Decider)
	tok, terr := hex.DecodeString(c.DeciderToken)
	if err != nil || !at.Addr().IsLoopback() || terr != nil || len(tok) != 32 {
		return fmt.Errorf("smolvm: %s: no decider to start it through", vm)
	}
	return nil
}

// services runs services.sh in a running machine, with args (servicesArgs
// for its sshd); a failure stops it.
func (s *Smolvm) services(ctx context.Context, vm string, args []string) error {
	script, err := s.containerScript("services.sh")
	if err != nil {
		return err
	}
	cmd := s.asRoot(ctx, vm, args...)
	cmd.Stdin = bytes.NewReader(script)
	if err := s.Runner.Run(ctx, msg.M{}, cmd); err != nil {
		if ctx.Err() == nil {
			_ = s.stop(ctx, vm)
		}
		return backend.Logged(msg.StartFailed(vm))
	}
	return nil
}

// Confined reports whether the machine runs on the process Start started
// through devvm's decider: its lifeline is the one recorded then, that
// process runs, and its allow lists are still network policy's (smolvm
// changes them on a running machine). A machine started any other way since
// (smolvm machine start, with a decider of someone else's), or whose allow
// list was widened, isn't.
func (s *Smolvm) Confined(ctx context.Context, vm string) (bool, error) {
	r, err := s.readRecord(vm)
	if err != nil || r.Confined == "" {
		return false, err
	}
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return false, notFound(vm, err)
	}
	if m.status() != backend.StatusRunning || !m.closed() {
		return false, nil
	}
	cur, err := s.lifeline(ctx, vm)
	return err == nil && runsOn(cur, r.Confined), err
}

// runsOn reports whether a machine whose lifeline holds cur runs on the
// process recorded as want: the same, and that process runs.
func runsOn(cur, want string) bool {
	if want == "" || cur != want {
		return false
	}
	first, _, _ := strings.Cut(cur, "\n")
	pid, err := strconv.Atoi(first)
	return err == nil && pid > 0 && alive(pid)
}

// lifeline returns what the machine's lifeline holds (its pid and start
// time), "" when there's none.
func (s *Smolvm) lifeline(ctx context.Context, vm string) (string, error) {
	path := s.Lifeline(ctx, vm)
	if path == "" {
		return "", nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // smolvm's own file for the machine
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// alive reports whether a process runs with that pid.
func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// Lifeline is the pid file of the process that runs the machine, in
// smolvm's folder for it: its first line is the pid, its second the start
// time. "" when smolvm can't say where that is.
func (s *Smolvm) Lifeline(ctx context.Context, vm string) string {
	dir := s.dataDir(ctx, vm)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "agent.pid")
}
