package lima

import (
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

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
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
// HTTP_PROXY and HTTPS_PROXY (and lowercase). Build networks (NetInternet,
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
		if sshPort, err = freePort(); err != nil {
			return err
		}
	}
	if sets := l.startSets(in.Config, c, sshPort); len(sets) > 0 {
		if err := l.edit(ctx, vm, sets...); err != nil {
			return err
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
	return l.recordStart(vm, c.Net)
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
			p.ProxyPort = c.ProxyPort
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

// startSets returns the --set edits that make a VM's config match how it
// starts, none when it already does: an edit gives the VM new ssh host keys
// at its next boot (cloud-init sees a new instance).
func (l *Lima) startSets(cfg *config, c backend.Confinement, sshPort int) []string {
	var sets []string
	if sshPort != 0 && cfg.SSH.LocalPort != sshPort {
		sets = append(sets, ".ssh.localPort="+strconv.Itoa(sshPort))
	}
	env := maps.Clone(c.Env)
	if env == nil {
		env = map[string]string{}
	}
	proxy := c.Net == backend.NetProxy
	if proxy {
		u := l.proxyURL(c)
		for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			env[k] = u
		}
	}
	if !maps.Equal(env, cfg.Env) && (len(env) > 0 || len(cfg.Env) > 0) {
		if len(env) == 0 {
			sets = append(sets, "del(.env)")
		} else {
			sets = append(sets, ".env = "+yqJSON(env))
		}
	}
	// Not the host's own proxy settings.
	propagates := cfg.PropagateProxyEnv == nil || *cfg.PropagateProxyEnv
	if proxy && propagates {
		sets = append(sets, ".propagateProxyEnv=false")
	} else if !proxy && !propagates {
		sets = append(sets, ".propagateProxyEnv=true")
	}
	// On Linux restrict=on drops Lima's DNS too, so lookups would time out;
	// a resolver nothing listens on fails at once. (On macOS the sandbox
	// makes DNS fail fast.)
	noDNS := proxy && l.OS == "linux"
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
	if c.Net == backend.NetInternet || c.Net == backend.NetNone {
		// The build's code can't take a port on host loopback that a host
		// program trusts.
		sets = append(sets, portsSet(backend.Ports{}))
	}
	var opts []string
	for _, s := range sets {
		opts = append(opts, "--set", s)
	}
	return opts
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

// freePort returns a loopback port nothing listens on now.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // a moment's listen
	if err != nil {
		return 0, err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port, nil
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

// Confined reports whether the VM runs on the hostagent Start started:
// anything else (limactl start) is a new hostagent, unconfined.
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
	want, _, _ := strings.Cut(strings.TrimSpace(string(rec)), " ")
	pid, err := strconv.Atoi(strings.TrimSpace(string(ha)))
	if err != nil || strconv.Itoa(pid) != want {
		return false, nil //nolint:nilerr // an unreadable pid is no hostagent of devvm's
	}
	return alive(pid), nil
}

// Lifeline is the VM's hostagent's pid file: the process that runs the VM
// and its network.
func (l *Lima) Lifeline(vm string) string { return l.haPIDFile(vm) }

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
