package netpolicy

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
)

// Proxy starts and stops VMs' proxies: devvm _proxy, one per VM, in the
// background, following the host process that runs the VM. It also
// chooses a new VM's network policy (Choose) and checks it holds
// (SelfCheck). NewProxy sets every field; tests change them.
type Proxy struct {
	Dirs    paths.Dirs
	Assets  fs.FS  // devvm's own files (assets.FS), whose built-in policies devvm network shows
	Self    string // devvm's binary, which runs as the proxy
	OS      string // the host's, as Go names it
	Secrets *secrets.Store
	// Start starts a process in the background and returns its pid
	// (daemon.Detach).
	Start daemon.Starter
	// Args returns a running process's arguments (platform.ProcessArgs).
	Args func(pid int) ([]string, error)
	// ExcludeFromBackup keeps a folder out of backups (macOS).
	ExcludeFromBackup func(ctx context.Context, dir string) error
	// InitCA readies a VM's CA and its next one (trust.InitCA).
	InitCA func(context.Context, paths.CA) error
	// Listening reports whether a program has a host loopback port.
	Listening func(ctx context.Context, port int) bool
	// PortRange is the range new ports come from (platform.ChoosablePorts,
	// which the zero value means too), which its allocator (Ports) has.
	PortRange platform.PortRange
	// Tick is how often a start looks for the proxy to listen, ten times;
	// a stop waits four Ticks for each signal to end it.
	Tick time.Duration
	// Signal sends a process a signal (Stop's: daemon.Signal).
	Signal func(pid int, sig syscall.Signal) error
	// Getenv reads devvm's environment: the host's proxy settings, which a
	// VM's proxy goes through over config.toml's (settings.HostProxy).
	Getenv func(string) string
	// What it prints, and asks ([Y/n]); Terminal reports whether it can
	// ask.
	Note, Warn func(msg.M)
	Confirm    func(msg.M) bool
	Terminal   func() bool
}

// UI is where the proxies print and ask.
type UI interface {
	Note(msg.M)
	Warn(msg.M)
	Confirm(q ...msg.M) bool
	StdinIsTerminal() bool
}

// NewProxy returns the proxies of devvm's folders, run by self (devvm's
// binary), with its own files (assets), printing and asking through u.
func NewProxy(d paths.Dirs, assets fs.FS, self string, store *secrets.Store, u UI) *Proxy {
	return &Proxy{
		Dirs: d, Assets: assets, Self: self, OS: runtime.GOOS, Secrets: store,
		Start: daemon.Detach, Args: platform.ProcessArgs, ExcludeFromBackup: platform.ExcludeFromBackup,
		InitCA:    trust.InitCA,
		Listening: ports.Listening, PortRange: platform.ChoosablePorts(), Tick: 500 * time.Millisecond, Signal: daemon.Signal, Getenv: os.Getenv,
		Note: u.Note, Warn: u.Warn, Confirm: func(m msg.M) bool { return u.Confirm(m) },
		Terminal: u.StdinIsTerminal,
	}
}

// proc is the VM's proxy as a background process: its pid file in the
// VM's run folder, and devvm _proxy -config with the VM's config in its
// arguments.
func (p *Proxy) proc(vm string) daemon.Proc {
	files := p.Dirs.VM(vm)
	return daemon.Proc{
		PIDFile: files.ProxyPID(), Args: p.Args, Signal: p.Signal,
		Is: func(args []string) bool { return daemon.HasFlag(args, "_proxy", "-config", files.ProxyConfig()) },
	}
}

// PID returns the pid of the VM's running proxy: the process its pid file
// names, if that still runs as the VM's proxy.
func (p *Proxy) PID(vm string) (int, bool) { return p.proc(vm).PID() }

// Stop stops the VM's proxy, if it runs: SIGTERM, then after two seconds
// SIGKILL, waiting up to two more for it to end. It reports whether no
// proxy of the VM's runs then.
func (p *Proxy) Stop(vm string) bool { return p.proc(vm).Stop(4 * p.Tick) }

// Unconfined is a VM starting without network policy: its proxy stopped,
// and its CAs deleted, keys and all, as nothing signs with them any more
// and the VM's setup takes their certificates out of the VM. Network
// policy turned on again makes new ones. The CAs stay while a proxy of
// the VM's still runs, which may sign with them.
func (p *Proxy) Unconfined(ctx context.Context, vm string) error {
	if !p.Stop(vm) {
		return nil
	}
	ca := p.Dirs.VM(vm).CA()
	if _, err := os.Lstat(ca.Dir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	// Under the CAs' lock, so it never takes them from a launch readying
	// them (InitCA).
	unlock, err := platform.LockFile(ctx, ca.Lock())
	if err != nil {
		return err
	}
	defer unlock()
	return os.RemoveAll(ca.Dir)
}

// Prepare readies a VM with network policy to start on b, or a running one
// to go on: the VM's own CA, its secret files, its ports, and its proxy
// running. It returns the confinement its backend starts it with. The proxy
// follows the host process that runs the VM (its Run.Lifeline): a VM
// started again gets a new proxy.
func (p *Proxy) Prepare(ctx context.Context, b backend.Backend, st *state.VM, running bool) (backend.Confinement, error) {
	vm := st.Name()
	none := backend.Confinement{}
	run, err := b.Running(ctx, vm)
	if err != nil {
		return none, err
	}
	lifeline := run.Lifeline
	// The VM's own CA, and the next one, which it trusts too (trust.InitCA).
	// Kept out of backups: a copy of a key elsewhere could impersonate any
	// site to the VM.
	ca := p.Dirs.VM(vm).CA()
	if err := p.InitCA(ctx, ca); err != nil {
		return none, msg.ProxyCAFailed(ca.Dir).Because(err)
	}
	if p.OS == "darwin" {
		_ = p.ExcludeFromBackup(ctx, ca.Dir)
	}
	key, err := Key(ctx, st)
	if err != nil {
		return none, err
	}
	salt, err := Salt(ctx, st)
	if err != nil {
		return none, err
	}
	if err := p.Secrets.WriteFiles(ctx, st, salt); err != nil {
		return none, err
	}
	// A stopped VM's proxy follows the hostagent it started with: a new
	// start gets a new one. A running VM's proxy starts again, on the same
	// port, when its config would differ from the one it started with
	// (stale).
	cfg, err := p.config(ctx, st, key, lifeline)
	if err != nil {
		return none, err
	}
	if stale := p.stale(vm, cfg); !running || stale {
		// Never the old one kept: it would go on with what it started with.
		if !p.Stop(vm) {
			return none, msg.ProxyStopFailed(vm)
		}
	}
	sp, err := p.Ports().SSH(ctx, st, running)
	if err != nil {
		return none, err
	}
	_, up := p.PID(vm)
	pp, err := p.proxyPort(ctx, st, up, running)
	if err != nil {
		return none, err
	}
	conf := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: pp, ProxyPassword: key, Env: Env, SSHPort: sp,
	}
	// The logs hold each VM's requests: the user's alone.
	for _, d := range []string{p.Dirs.Run(), p.Dirs.VM(vm).Run(), p.Dirs.VM(vm).Logs()} {
		if err := state.PrivateDir(d); err != nil {
			return none, err
		}
	}
	if up {
		return conf, nil
	}
	cfg.Listen = listenOn(pp)
	return conf, p.start(vm, cfg)
}

// config is the config the VM's proxy would start with now, listening on
// the proxy port the VM has (none: port 0), with its key (the password)
// and following its lifeline. It holds everything that, changed, starts a
// running VM's proxy again: its policies (tools added or removed, a layer
// of the user's), devvm's binary (an upgrade, which may change the
// built-in policies' rules), the VM's CAs (replaced, so it would mint
// certificates the VM no longer trusts after its next setup), the user's
// CAs, which it trusts for its own connections, the company proxy it goes
// through (upstream), and the shared Nix cache's cap.
func (p *Proxy) config(ctx context.Context, st *state.VM, key, lifeline string) (proxy.Config, error) {
	vm, files := st.Name(), p.Dirs.VM(st.Name())
	ca := files.CA()
	builtin, policies := Policies(p.Dirs, vm, st.Image, tools.Names(st))
	for _, m := range PolicyWarnings(policies) {
		p.Warn(m)
	}
	cas, err := trust.Read(p.Dirs.TrustedCAs())
	if err != nil {
		return proxy.Config{}, err
	}
	upstream, err := p.Upstream(ctx)
	if err != nil {
		return proxy.Config{}, err
	}
	pp, err := ports.Of(st)
	if err != nil {
		return proxy.Config{}, err
	}
	bin, _ := image.SHA256File(p.Self)
	caSum, _ := image.SHA256File(ca.Cert())
	nextSum, _ := image.SHA256File(ca.NextCert())
	c := proxy.Config{
		Listen: listenOn(pp.Host.Proxy), VM: vm, Password: key,
		BuiltinPolicies: builtin, Policies: policies,
		Log: files.EventLog(), ProxyLog: files.ProxyLog(), Lifeline: lifeline,
		CA: ca.Cert(), CAKey: ca.Key(), CANext: ca.NextCert(), CANextKey: ca.NextKey(),
		Secrets:     files.Secrets(),
		UpstreamCAs: string(cas.Bundle()), Upstream: upstream,
		Links: files.LinkList(), LinkSockets: p.Dirs.Run(),
		DevvmSHA256: bin, CASHA256: caSum, CANextSHA256: nextSum,
	}
	c.NixCache, c.NixCacheMax = p.nixCache(st)
	return c, nil
}

// Upstream is the company proxy a VM's proxy, or a build's, goes through:
// the environment's, else config.toml's, checked, with config.toml's
// password saved (settings.CompanyProxy.Upstream).
func (p *Proxy) Upstream(ctx context.Context) (*proxy.Upstream, error) {
	return p.Company().Upstream(ctx)
}

// Company is the company proxy as devvm's environment and config.toml
// have it now, with config.toml's password from p's secrets.
func (p *Proxy) Company() *settings.CompanyProxy {
	return settings.NewCompanyProxy(p.Dirs, p.Getenv, p.Secrets.ProxyPassword)
}

// stale reports whether the VM's proxy last started with another config
// than cfg (its Sum): one that's gone or can't be read counts as another.
// It warns when that proxy went through a company proxy and cfg goes
// through none, as a shell without the proxy settings would start it.
func (p *Proxy) stale(vm string, cfg proxy.Config) bool {
	old, err := proxy.ReadConfig(p.Dirs.VM(vm).ProxyConfig())
	if err != nil {
		return true
	}
	if old.Upstream != nil && cfg.Upstream == nil {
		p.Warn(msg.UpstreamProxyGone(p.Dirs.ConfigFile(), vm))
	}
	return old.Sum() != cfg.Sum()
}

// listenOn is the proxy's address for port, on the host's loopback.
func listenOn(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }

// Ports is the allocator of VMs' host ports, which asks Listening
// whether a program has one.
func (p *Proxy) Ports() ports.Allocator {
	return ports.Allocator{Dirs: p.Dirs, Listening: p.Listening, Range: p.PortRange}
}

// proxyPort returns the port of the VM's proxy: a new free one (Ports)
// when it has none, or when its proxy doesn't run and another program
// took it, unless the VM runs: its backend holds the port it started
// with.
func (p *Proxy) proxyPort(ctx context.Context, st *state.VM, up, running bool) (int, error) {
	pp, err := ports.Of(st)
	if err != nil {
		return 0, err
	}
	n := pp.Host.Proxy
	if up || n != 0 && !p.Listening(ctx, n) {
		return n, nil
	}
	if running && n != 0 {
		return 0, msg.ProxyPortTaken(strconv.Itoa(n), st.Name())
	}
	// One another devvm chose meanwhile is kept while it's free.
	return p.Ports().Choose(ctx, st, ports.Proxy, func(cur int, _ ports.Ports) bool { return cur != n && !p.Listening(ctx, cur) })
}

// RefreshSecrets writes again the secret files of the VMs named (none:
// every VM) whose proxy has them, so a token saved, deleted, bound or
// unbound applies at once: a proxy reads its files again when they change,
// and looks its values up afresh. Its logins' bindings stay as its start
// wrote them.
func (p *Proxy) RefreshSecrets(ctx context.Context, vms ...string) error {
	if len(vms) == 0 {
		all, err := state.VMNames(p.Dirs)
		if err != nil {
			return err
		}
		vms = all
	}
	for _, vm := range vms {
		if _, err := os.Stat(p.Dirs.VM(vm).Secrets()); err != nil {
			continue
		}
		st, err := state.ReadVM(p.Dirs, vm)
		if err != nil || !st.Exists() {
			continue // not a VM devvm still has
		}
		n, err := Of(st)
		if err != nil || n.Made.ProxySalt == "" {
			continue
		}
		if err := p.Secrets.RefreshFiles(ctx, st, n.Made.ProxySalt); err != nil {
			return err
		}
	}
	return nil
}

// start writes cfg, the VM's proxy's config, and starts the proxy with
// it, devvm _proxy -config FILE, waiting for it to listen. The config holds
// the password and the company proxy's, so it's the user's alone, and in
// no process's arguments, which other users can see.
func (p *Proxy) start(vm string, cfg proxy.Config) error {
	files := p.Dirs.VM(vm)
	if err := p.writeConfig(vm, cfg); err != nil {
		return err
	}
	out, err := os.OpenFile(cfg.ProxyLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	status := files.ProxyStatus()
	args := []string{"_proxy", "-config", files.ProxyConfig()}
	pid, err := p.proc(vm).Start(p.Start, daemon.Cmd{Path: p.Self, Args: args, Out: out, Status: status})
	if err != nil {
		return err
	}
	// Started means this proxy says it listens on the port, and still runs:
	// another program that took the port first would otherwise get the
	// VM's traffic.
	if said, ok := p.proc(vm).Ready(status, pid, p.Tick, 10); ok && said == cfg.Listen {
		return nil
	}
	return msg.ProxyStartFailed(cfg.ProxyLog, vm)
}

// writeConfig writes the VM's proxy's config whole, mode 600, so a proxy
// never reads half of it, and two devvms writing it at once leave one of
// theirs.
func (p *Proxy) writeConfig(vm string, cfg proxy.Config) error {
	return state.WriteFile(p.Dirs.VM(vm).ProxyConfig(), cfg.Encode())
}

// nixCache is the proxy's shared Nix cache and its cap: only for a VM
// whose image has the nix layer, with the cap nix-cache.size gives (its
// default when the retention settings can't be read, with a warning).
func (p *Proxy) nixCache(st *state.VM) (string, int64) {
	if !image.HasLayer(st.Image, image.NixLayer) {
		return "", 0
	}
	def, err := settings.Read(p.Dirs)
	if err != nil {
		p.Warn(msg.RetentionUnreadable().Because(err))
	}
	return p.Dirs.NixCache(), def.Retention.NixCacheCap()
}
