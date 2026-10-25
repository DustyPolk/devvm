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
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"

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
// in no process's arguments. A machine that
// forwards an SSH agent forwards c.AgentSocket, or the user's
// (SSH_AUTH_SOCK). With c.SSH its ssh is forwarded (sshForward) and
// services.sh runs its sshd; without, the forward goes.
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
	// The VM's ssh: forwarded while it's asked for, its sshd run then.
	var sshd []string
	if c.SSH {
		port, err := s.sshForward(vm, c, r)
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
	r.Mediated, r.Confined = r.Mediated || proxy, ""
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
	if r.Forwarding {
		sock := c.AgentSocket
		if sock == "" {
			sock = s.Getenv("SSH_AUTH_SOCK")
		}
		if sock == "" {
			return msg.SmolvmNoAgent(vm)
		}
		cmd.Env = setEnv(cmd.Env, "SSH_AUTH_SOCK="+sock)
	} else {
		cmd.Env = setEnv(cmd.Env, "SSH_AUTH_SOCK")
	}
	if err := s.Runner.Run(ctx, msg.Starting(vm), cmd); err != nil {
		return backend.Logged(msg.StartFailed(vm))
	}
	if proxy {
		if r.Confined, err = s.lifeline(vm); err != nil {
			return err
		}
		if err := s.writeRecord(vm, r); err != nil {
			return err
		}
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
	if r.SSHPort == 0 {
		return nil
	}
	r.SSHUp, r.NoSSHD = !noSSHD, noSSHD
	return s.writeRecord(vm, r)
}

// sshdProbe answers "sshd" when the machine's image has sshd, else "none".
const sshdProbe = `if [ -x /usr/sbin/sshd ] || command -v sshd >/dev/null 2>&1; then echo sshd; else echo none; fi`

// lacksSSHD reports whether a running machine's image has no sshd (made
// before devvm installed one), asked before services.sh runs, so no other
// failure reads as it; one that can't say stops it.
func (s *Smolvm) lacksSSHD(ctx context.Context, vm string) (bool, error) {
	out, err := s.Runner.Output(s.smolvm(ctx, "machine", "exec", "--name", vm, "--", "sh", "-c", sshdProbe))
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
	cmd := s.asRoot(ctx, vm, args...)
	cmd.Stdin = bytes.NewReader(servicesScript)
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
	cur, err := s.lifeline(vm)
	if err != nil || cur != r.Confined {
		return false, err
	}
	first, _, _ := strings.Cut(cur, "\n")
	pid, err := strconv.Atoi(first)
	if err != nil || pid <= 0 {
		return false, nil //nolint:nilerr // an unreadable pid is no process of devvm's start
	}
	return alive(pid), nil
}

// lifeline returns what the machine's lifeline holds (its pid and start
// time), "" when there's none.
func (s *Smolvm) lifeline(vm string) (string, error) {
	path := s.Lifeline(vm)
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
func (s *Smolvm) Lifeline(vm string) string {
	out, err := s.Runner.Output(s.smolvm(context.Background(), "machine", "data-dir", "--name", vm))
	dir := strings.TrimSpace(string(out))
	if err != nil || !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Join(dir, "agent.pid")
}
