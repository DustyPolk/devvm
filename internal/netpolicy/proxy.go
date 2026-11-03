package netpolicy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	// InitCA makes devvm's CA in dir unless a current one is there.
	InitCA func(dir string) error
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
	// VM's proxy goes through (backend.ReadHostProxy).
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
		InitCA: func(dir string) error {
			return proxy.Main([]string{"-init-ca", dir}, proxy.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
		},
		Listening: ports.Listening, PortRange: platform.ChoosablePorts(), Tick: 500 * time.Millisecond, Signal: daemon.Signal, Getenv: os.Getenv,
		Note: u.Note, Warn: u.Warn, Confirm: func(m msg.M) bool { return u.Confirm(m) },
		Terminal: u.StdinIsTerminal,
	}
}

// run is one of the VM's proxy's files in its run folder (paths.VM.Proxy).
func (p *Proxy) run(vm, ext string) string { return p.Dirs.VM(vm).Proxy(ext) }

// proc is the VM's proxy as a background process: its pid file in the
// VM's run folder, and devvm _proxy -vm VM in its arguments.
func (p *Proxy) proc(vm string) daemon.Proc {
	return daemon.Proc{
		PIDFile: p.run(vm, ".pid"), Args: p.Args, Signal: p.Signal,
		Is: func(args []string) bool { return daemon.HasFlag(args, "_proxy", "-vm", vm) },
	}
}

// PID returns the pid of the VM's running proxy: the process its pid file
// names, if that still runs as the VM's proxy.
func (p *Proxy) PID(vm string) (int, bool) { return p.proc(vm).PID() }

// Stop stops the VM's proxy, if it runs: SIGTERM, then after two seconds
// SIGKILL, waiting up to two more for it to end. It reports whether no
// proxy of the VM's runs then.
func (p *Proxy) Stop(vm string) bool { return p.proc(vm).Stop(4 * p.Tick) }

// Prepare readies a VM with network policy to start on b, or a running one
// to go on: devvm's CA, the VM's secret files, its ports, and its proxy
// running. It returns the confinement its backend starts it with. The proxy
// follows the host process that runs the VM (b's Lifeline): a VM started
// again gets a new proxy.
func (p *Proxy) Prepare(ctx context.Context, b backend.Backend, st *state.VM, running bool) (backend.Confinement, error) {
	vm := st.Name()
	lifeline := b.Lifeline(ctx, vm)
	none := backend.Confinement{}
	// The CA lasts 30 days: a new one is made when it has under a week left
	// (VMs trust it from their next start). Kept out of backups: a copy of
	// its key elsewhere could impersonate any site to VMs.
	ca := p.Dirs.CA()
	if err := state.PrivateDir(ca); err != nil {
		return none, err
	}
	if err := p.InitCA(ca); err != nil {
		return none, msg.ProxyCAFailed(ca)
	}
	if p.OS == "darwin" {
		_ = p.ExcludeFromBackup(ctx, ca)
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
	// start gets a new one. One whose policies changed (tools added or
	// removed), from another devvm binary (an upgrade, which may change
	// the built-in policies' rules), or holding a CA
	// since replaced (which mints certificates the VM no longer trusts
	// after its next setup) starts again too, on the same port.
	builtin, files := Policies(p.Dirs, vm, st.Image, tools.Names(st))
	for _, m := range PolicyWarnings(files) {
		p.Warn(m)
	}
	policies := policyList{builtin: strings.Join(builtin, ","), files: strings.Join(files, ",")}
	bin, _ := image.SHA256File(p.Self)
	caSum, _ := image.SHA256File(filepath.Join(ca, "ca.pem"))
	// The user's CAs, which it trusts for its own connections: one whose
	// folder changed starts again too.
	cas, err := trust.Read(p.Dirs.TrustedCAs())
	if err != nil {
		return none, err
	}
	// The company proxy it goes through, from devvm's environment: one
	// whose settings changed starts again too.
	upstream, err := backend.ReadHostProxy(p.Getenv).Upstream()
	if err != nil {
		return none, err
	}
	if !running || p.stale(vm, policies, bin, caSum, cas.Bundle(), upstream) {
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
	if err := p.writeUpstream(vm, cas.Bundle(), upstream); err != nil {
		return none, err
	}
	return conf, p.start(st, conf, policies, bin, caSum, lifeline)
}

// policyList is a VM's policies (Policies) as its proxy's flags list
// them: the built-in ones (-builtin-policy) and the user's files
// (-policy), each comma-separated.
type policyList struct{ builtin, files string }

// record is what the run folder keeps of the policies a proxy started
// with: both lists, a line each, so a change to either starts it again.
func (l policyList) record() string { return l.builtin + "\n" + l.files }

// stale reports whether the VM's proxy started with other settings than
// these: its policies (tools added or removed), devvm's binary (an
// upgrade), devvm's CA (replaced, so it mints certificates the VM no
// longer trusts after its next setup), the user's CAs, or the company
// proxy it goes through.
func (p *Proxy) stale(vm string, policies policyList, bin, caSum string, cas, upstream []byte) bool {
	oldCAs, _ := os.ReadFile(p.run(vm, ".cas"))
	oldUp, _ := os.ReadFile(p.run(vm, ".upstream"))
	return read(p.run(vm, ".policy")) != policies.record() || read(p.run(vm, ".bin")) != bin || read(p.run(vm, ".ca")) != caSum ||
		!bytes.Equal(oldCAs, cas) || !bytes.Equal(oldUp, upstream)
}

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

// writeUpstream writes the user's CAs (-upstream-cas) and the company
// proxy (-upstream, its password with it, so owner-only) where the VM's
// proxy reads them as it starts, or removes each file for none.
func (p *Proxy) writeUpstream(vm string, cas, upstream []byte) error {
	for _, f := range []struct {
		ext  string
		data []byte
	}{{".cas", cas}, {".upstream", upstream}} {
		path := p.run(vm, f.ext)
		if len(f.data) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		if err := state.WriteFile(path, f.data); err != nil {
			return err
		}
	}
	return nil
}

// start starts the VM's proxy as conf has it, and waits for it to listen.
func (p *Proxy) start(st *state.VM, conf backend.Confinement, policies policyList, bin, caSum, lifeline string) error {
	vm := st.Name()
	// The password goes in a file: a process's arguments are visible to
	// other users.
	auth := p.run(vm, ".auth")
	if err := state.WriteFile(auth, []byte(conf.ProxyPassword+"\n")); err != nil {
		return err
	}
	files := p.Dirs.VM(vm)
	logPath := files.ProxyLog()
	out, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // devvm's own log
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	ca := p.Dirs.CA()
	addr := "127.0.0.1:" + strconv.Itoa(conf.ProxyPort)
	args := []string{
		"_proxy", "-listen", addr, "-vm", vm, "-builtin-policy", policies.builtin, "-policy", policies.files, "-log", files.EventLog(), "-proxy-log", logPath, "-lifeline", lifeline,
		"-ca", filepath.Join(ca, "ca.pem"), "-cakey", filepath.Join(ca, "ca.key"),
		"-secrets", files.Secrets(), "-secret-cmd", p.Self, "-auth-file", auth, "-links", files.LinkList(), "-link-sockets", p.Dirs.Run(),
	}
	for _, f := range [][2]string{{".cas", "-upstream-cas"}, {".upstream", "-upstream"}} {
		if _, err := os.Stat(p.run(vm, f[0])); err == nil {
			args = append(args, f[1], p.run(vm, f[0]))
		}
	}
	args = append(args, p.nixCacheArgs(st)...)
	status := p.run(vm, ".status")
	pid, err := p.proc(vm).Start(p.Start, daemon.Cmd{Path: p.Self, Args: args, Out: out, Status: status})
	if err != nil {
		return err
	}
	for _, f := range [][2]string{{".policy", policies.record()}, {".bin", bin}, {".ca", caSum}} {
		if err := state.WriteFile(p.run(vm, f[0]), []byte(f[1]+"\n")); err != nil {
			return err
		}
	}
	// Started means this proxy says it listens on the port, and still runs:
	// another program that took the port first would otherwise get the
	// VM's traffic.
	if said, ok := p.proc(vm).Ready(status, pid, p.Tick, 10); ok && said == addr {
		return nil
	}
	return msg.ProxyStartFailed(logPath, vm)
}

// nixCacheArgs are the proxy's flags for the shared Nix cache: only for a
// VM whose image has the nix layer, with the cap nix-cache.size gives (its
// default when the retention settings can't be read, with a warning).
func (p *Proxy) nixCacheArgs(st *state.VM) []string {
	if !image.HasLayer(st.Image, image.NixLayer) {
		return nil
	}
	def, err := settings.Read(p.Dirs)
	if err != nil {
		p.Warn(msg.RetentionUnreadable().Because(err))
	}
	return []string{"-nix-cache", p.Dirs.NixCache(), "-nix-cache-max", strconv.FormatInt(def.Retention.NixCacheCap(), 10)}
}

func read(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own run folder
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
