package lima

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// proxyHostDarwin is host loopback as a guest sees it on macOS: Lima's
// user-mode network's gateway.
const proxyHostDarwin = "192.168.5.2"

func (l *Lima) profileFile(vm string) string { return filepath.Join(l.Dirs.Run(), vm+".sb") }

// startedFile records the hostagent a VM was started on, and how it was
// confined: "<pid> <net>".
func (l *Lima) startedFile(vm string) string { return filepath.Join(l.Dirs.Run(), vm+".started") }

func (l *Lima) haPIDFile(vm string) string { return filepath.Join(l.Dirs.LimaVM(vm), "ha.pid") }

// Start starts a stopped VM confined as c asks, and returns once it
// answers over ssh. A running VM is left as it is.
//
// The VM's guest environment (Lima's env:, written to /etc/environment on
// every boot) is c.Env, plus, for NetProxy, the proxy's address in
// HTTP_PROXY and HTTPS_PROXY (and lowercase), its password written into
// lima.yaml by devvm, never in limactl's arguments. Build networks (NetInternet,
// NetNone) forward no ports.
func (l *Lima) Start(ctx context.Context, vm string, c backend.Confinement) error {
	if err := l.checkGlobalConfigs(); err != nil {
		return err
	}
	in, err := l.inspect(ctx, vm)
	if err != nil {
		return err
	}
	// devvm never gives a VM a network of its own: one would reach
	// around the proxy.
	if len(in.Config.Networks) > 0 {
		return msg.VMHasNetworks(filepath.Join(l.Dirs.LimaVM(vm), "lima.yaml"), vm)
	}
	if backend.Status(in.Status) == backend.StatusRunning {
		return nil
	}
	sshPort := c.SSHPort
	if sshPort == 0 && l.OS == "darwin" && c.Net != backend.NetDirect {
		// The profile names the port, so Lima can't pick it.
		if sshPort, err = freePort(ctx); err != nil {
			return err
		}
	}
	if sets, fill := l.startSets(in.Config, c, sshPort); len(sets) > 0 {
		if err := l.edit(ctx, vm, sets...); err != nil {
			return err
		}
		if fill {
			if err := l.fillPassword(vm, c); err != nil {
				return err
			}
		}
	}
	// Its profile, wrapper and records: the user's alone.
	if err := os.MkdirAll(l.Dirs.Run(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(l.Dirs.Run(), 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
		return err
	}
	wrap, env, err := l.confine(vm, c, sshPort, in.backend().AgentForwarding)
	if err != nil {
		return err
	}
	if err := os.Remove(l.startedFile(vm)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cmd := l.limactl(ctx, "start", "--tty=false", vm)
	if len(wrap) > 0 {
		// sandbox-exec runs limactl by its path, in the same folder.
		cmd.Args = append(slices.Clone(wrap), append([]string{cmd.Path}, cmd.Args[1:]...)...)
		cmd.Path = wrap[0]
	}
	cmd.Env = setEnv(cmd.Env, env...)
	if err := l.Runner.Run(ctx, msg.Starting(vm), cmd); err != nil {
		if err := l.degraded(ctx, vm); err != nil {
			return err
		}
	}
	if err := l.recordStart(vm, c.Net); err != nil {
		return err
	}
	// A project VM's ports, through devvm's forwarder (ports.go).
	if c.HostKey != nil && !l.Forwards.start(ctx, vm, c, l.SSH, l.haPIDFile(vm)) {
		l.Warn(msg.PortsForwardFailed(l.Forwards.Log(vm), vm))
	}
	return nil
}

// degraded decides on a start Lima reported as failed. Lima reports that
// for a VM that's running but degraded (one of its own boot scripts
// failed), which is usable: carry on with a warning. Unless it doesn't
// answer over ssh: then it never booted (QEMU can keep running a guest
// stuck in its firmware), and it's stopped.
func (l *Lima) degraded(ctx context.Context, vm string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	in, err := l.inspect(ctx, vm)
	if err != nil || backend.Status(in.Status) != backend.StatusRunning {
		return backend.Logged(msg.StartFailed(vm))
	}
	sctx, cancel := context.WithTimeout(ctx, l.SSHTimeout)
	defer cancel()
	if err := l.Exec(sctx, vm, []string{"true"}, backend.ExecOpts{}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_ = l.Runner.Run(ctx, msg.Stopping(vm), l.limactl(ctx, "stop", "--force", vm))
		return backend.Logged(msg.StartNoSSH(filepath.Join(l.Dirs.LimaVM(vm), "serial*.log"), vm))
	}
	l.Warn(msg.StartedWithWarnings(vm))
	return nil
}

// confine returns how limactl start runs confined: the command it runs
// under (sandbox-exec and its profile; none), and the environment it adds
// (KEY=value, or a bare KEY to remove).
func (l *Lima) confine(vm string, c backend.Confinement, sshPort int, forwarding bool) (wrap, env []string, err error) {
	// Lima's processes forward the agent they find at start.
	if c.AgentSocket != "" {
		env = append(env, "SSH_AUTH_SOCK="+c.AgentSocket)
	}
	agent := c.AgentSocket
	if agent == "" {
		agent = l.Getenv("SSH_AUTH_SOCK")
	}
	switch l.OS {
	case "darwin":
		p := profile{LimaDir: l.Dirs.LimaVM(vm), SSHPort: sshPort}
		switch c.Net {
		case backend.NetDirect:
			return nil, env, nil
		case backend.NetProxy:
			p.ProxyPort, p.NoBinds = c.ProxyPort, true
			if !forwarding {
				agent = ""
			}
			p.AgentSocket = agent
		case backend.NetNone:
			p.AgentSocket = agent
		case backend.NetInternet:
			p.Internet = true
		}
		if err := writeProfile(l.profileFile(vm), p); err != nil {
			return nil, nil, err
		}
		return []string{sandboxExec, "-f", l.profileFile(vm)}, env, nil
	case "linux":
		switch c.Net {
		case backend.NetDirect, backend.NetInternet:
			// QEMU can't keep the internet and drop host loopback.
			return nil, env, nil
		case backend.NetProxy, backend.NetNone:
		}
		w, err := writeWrapper(filepath.Join(l.Dirs.Run(), "qemu"), l.Self, l.Arch)
		if err != nil {
			return nil, nil, err
		}
		env = append(env, qemuEnvVar(l.Arch)+"="+w)
		if c.Net == backend.NetProxy {
			env = append(env, GuestFwdEnv+"="+guestFwdCmd(l.Self, "127.0.0.1:"+strconv.Itoa(c.ProxyPort)))
		} else {
			env = append(env, GuestFwdEnv) // nothing forwarded
		}
		return nil, env, nil
	}
	return nil, nil, msg.LimaHostUnsupported(l.OS)
}

// passwordStandIn stands for the proxy's password in the edit that sets
// the VM's environment: limactl's arguments are visible to other users, so
// devvm writes the password into the VM's lima.yaml itself (fillPassword).
const passwordStandIn = "devvm-proxy-password" //nolint:gosec // a stand-in, no password

// startSets returns the --set edits that make a VM's config match how it
// starts, none when it already does: an edit gives the VM new ssh host keys
// at its next boot (cloud-init sees a new instance). fill reports that the
// edits hold passwordStandIn for fillPassword to replace.
func (l *Lima) startSets(cfg *config, c backend.Confinement, sshPort int) (opts []string, fill bool) {
	var sets []string
	if sshPort != 0 && cfg.SSH.LocalPort != sshPort {
		sets = append(sets, ".ssh.localPort="+strconv.Itoa(sshPort))
	}
	proxy := c.Net == backend.NetProxy
	if env := l.guestEnv(c); !maps.Equal(env, cfg.Env) && (len(env) > 0 || len(cfg.Env) > 0) {
		if len(env) == 0 {
			sets = append(sets, "del(.env)")
		} else {
			stand := c
			stand.ProxyPassword = passwordStandIn
			sets = append(sets, ".env = "+yqJSON(l.guestEnv(stand)))
			fill = proxy && c.ProxyUser != ""
		}
	}
	// The host's own proxy settings only for a VM on Lima's own network:
	// Lima writes them into the guest's /etc/environment, which a build
	// VM's image would keep, password and all (its scripts get them on
	// their input instead: buildproxy.go), and a VM with network policy
	// goes through devvm's proxy.
	direct := c.Net == backend.NetDirect
	propagates := cfg.PropagateProxyEnv == nil || *cfg.PropagateProxyEnv
	if !direct && propagates {
		sets = append(sets, ".propagateProxyEnv=false")
	} else if direct && !propagates {
		sets = append(sets, ".propagateProxyEnv=true")
	}
	// Names resolve in the proxy. On Linux restrict=on drops Lima's DNS
	// too, so lookups would time out; on macOS Lima's host resolver binds
	// loopback ports, which the sandbox denies (seatbelt.go). A resolver
	// nothing listens on fails at once.
	noDNS := proxy
	resolver := cfg.HostResolver.Enabled == nil || *cfg.HostResolver.Enabled
	switch {
	case noDNS && resolver:
		sets = append(sets, ".hostResolver.enabled=false")
	case !noDNS && !resolver:
		sets = append(sets, "del(.hostResolver)")
	}
	switch {
	case noDNS && !slices.Equal(cfg.DNS, []string{"127.0.0.1"}):
		sets = append(sets, `.dns=["127.0.0.1"]`)
	case !noDNS && len(cfg.DNS) > 0:
		sets = append(sets, "del(.dns)")
	}
	// Lima forwards no port: devvm's forwarder does (ports.go).
	if !forwardsNone(cfg.PortForwards) {
		sets = append(sets, noForwardsSet())
	}
	for _, s := range sets {
		opts = append(opts, "--set", s)
	}
	return opts, fill
}

// guestEnv is the VM's guest environment as c has it: c.Env, and for
// NetProxy the proxy's address.
func (l *Lima) guestEnv(c backend.Confinement) map[string]string {
	env := maps.Clone(c.Env)
	if env == nil {
		env = map[string]string{}
	}
	if c.Net == backend.NetProxy {
		u := l.proxyURL(c)
		for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			env[k] = u
		}
	}
	return env
}

// fillPassword replaces the proxy URL with passwordStandIn in the VM's
// lima.yaml, as limactl edit wrote it, with the real one: the file, unlike
// a process's arguments, is the user's alone (devvm writes it 0600).
func (l *Lima) fillPassword(vm string, c backend.Confinement) error {
	path := filepath.Join(l.Dirs.LimaVM(vm), "lima.yaml")
	data, err := os.ReadFile(path) //nolint:gosec // the VM's own config, in devvm's Lima home
	if err != nil {
		return err
	}
	stand := c
	stand.ProxyPassword = passwordStandIn
	from, to := l.proxyURL(stand), l.proxyURL(c)
	// The URL goes into YAML as it is: only characters no YAML scalar
	// treats specially.
	if strings.Trim(to, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~%:/@") != "" {
		return fmt.Errorf("lima: %s: the proxy's URL can't go into lima.yaml", vm)
	}
	if !bytes.Contains(data, []byte(from)) {
		return fmt.Errorf("lima: %s: lima.yaml has no proxy settings to fill in", vm)
	}
	return state.WriteFile(path, bytes.ReplaceAll(data, []byte(from), []byte(to)))
}

// proxyURL is devvm's proxy as the guest reaches it, with the VM's user and
// password.
func (l *Lima) proxyURL(c backend.Confinement) string {
	host := guestFwdAddr
	if l.OS == "darwin" {
		host = net.JoinHostPort(proxyHostDarwin, strconv.Itoa(c.ProxyPort))
	}
	u := url.URL{Scheme: "http", Host: host}
	if c.ProxyUser != "" {
		u.User = url.UserPassword(c.ProxyUser, c.ProxyPassword)
	}
	return u.String()
}

// freePort returns a loopback port nothing listens on now, on any address
// (platform.PortFree).
func freePort(ctx context.Context) (int, error) {
	for range 20 {
		ln, err := platform.ListenLoopback(ctx, "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		if platform.PortFree(ctx, port) {
			return port, nil
		}
	}
	return 0, errors.New("lima: no free port on the host's loopback")
}

// recordStart notes the hostagent Lima started for the VM, which a
// confined start started confined (a process can't leave its sandbox).
func (l *Lima) recordStart(vm string, n backend.Net) error {
	pid, err := os.ReadFile(l.haPIDFile(vm))
	if errors.Is(err, os.ErrNotExist) {
		return nil // no hostagent to record: Confined says no
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(l.Dirs.Run(), 0o700); err != nil {
		return err
	}
	return writeFile(l.startedFile(vm), fmt.Appendf(nil, "%s %d\n", strings.TrimSpace(string(pid)), n), 0o600)
}

// Confined reports whether the VM runs on the hostagent Start started
// with NetProxy: anything else (limactl start) is a new hostagent,
// unconfined.
func (l *Lima) Confined(_ context.Context, vm string) (bool, error) {
	rec, err := os.ReadFile(l.startedFile(vm))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	ha, err := os.ReadFile(l.haPIDFile(vm))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	want, n, _ := strings.Cut(strings.TrimSpace(string(rec)), " ")
	pid, err := strconv.Atoi(strings.TrimSpace(string(ha)))
	if err != nil || strconv.Itoa(pid) != want || n != strconv.Itoa(int(backend.NetProxy)) {
		return false, nil //nolint:nilerr // an unreadable pid is no hostagent of devvm's
	}
	return alive(pid), nil
}

// StartID is a running VM's hostagent: its pid file, which each start
// writes anew, by the pid it holds and when it was written; "" without
// one (Lima removes it when the VM stops).
func (l *Lima) StartID(_ context.Context, vm string) (string, error) {
	path := l.haPIDFile(vm)
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	pid, err := os.ReadFile(path) //nolint:gosec // Lima's own pid file for the VM
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(pid)) + " " + fi.ModTime().UTC().Format(time.RFC3339Nano), nil
}

// Lifeline is the VM's hostagent's pid file: the process that runs the VM
// and its network.
func (l *Lima) Lifeline(_ context.Context, vm string) string { return l.haPIDFile(vm) }

// ProxyURL is devvm's proxy as a VM started with c reaches it.
func (l *Lima) ProxyURL(c backend.Confinement) string {
	if c.Net != backend.NetProxy {
		return ""
	}
	return l.proxyURL(c)
}

// alive reports whether a process runs with that pid.
func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
