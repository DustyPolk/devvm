package netpolicy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/proxy"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/trust"
)

// Proxy starts and stops VMs' proxies: devvm _proxy, one per VM, in the
// background, following the host process that runs the VM. It also
// chooses a new VM's network policy (Choose) and checks it holds
// (SelfCheck). NewProxy sets every field; tests change them.
type Proxy struct {
	Dirs    paths.Dirs
	Assets  fs.FS  // devvm's own files (assets.FS): layers/, shared-tools/
	Self    string // devvm's binary, which runs as the proxy
	OS      string // the host's, as Go names it
	Secrets *secrets.Store
	// Start starts path with args in the background, its output in out,
	// and returns its pid (platform.StartDetached).
	Start func(path string, args []string, out *os.File) (int, error)
	// Args returns a running process's arguments (platform.ProcessArgs).
	Args func(pid int) ([]string, error)
	// ExcludeFromBackup keeps a folder out of backups (macOS).
	ExcludeFromBackup func(ctx context.Context, dir string) error
	// InitCA makes devvm's CA in dir unless a current one is there.
	InitCA func(dir string) error
	// Listening reports whether a program has a host loopback port.
	Listening func(ctx context.Context, port int) bool
	// Tick is how often a start looks for the proxy to listen, ten times;
	// a stop waits four Ticks for each signal to end it.
	Tick time.Duration
	// Signal sends a process a signal (Stop's).
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
		Start: platform.StartDetached, Args: platform.ProcessArgs, ExcludeFromBackup: platform.ExcludeFromBackup,
		InitCA: func(dir string) error {
			return proxy.Main([]string{"-init-ca", dir}, proxy.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
		},
		Listening: listening, Tick: 500 * time.Millisecond, Signal: signal, Getenv: os.Getenv,
		Note: u.Note, Warn: u.Warn, Confirm: func(m msg.M) bool { return u.Confirm(m) },
		Terminal: u.StdinIsTerminal,
	}
}

func (p *Proxy) run(vm, ext string) string { return filepath.Join(p.Dirs.Run(), vm+".proxy"+ext) }

// PolicyFiles returns the policy files of the VM vm, of image img, its
// proxy reads (PolicyFiles), devvm's own written out for it.
func (p *Proxy) PolicyFiles(vm, img string) ([]string, error) {
	assets, err := state.WriteAssets(p.Dirs, p.Assets)
	if err != nil {
		return nil, err
	}
	return PolicyFiles(p.Dirs, assets, vm, img), nil
}

// AssetsInUse returns the folders of devvm's files written out
// (state.WriteAssets) that VMs' proxies were started with, by name: kept
// while a proxy may still read its policy files there.
func AssetsInUse(d paths.Dirs) map[string]bool {
	inUse := map[string]bool{}
	started, _ := filepath.Glob(filepath.Join(d.Run(), "*.proxy.policy"))
	for _, f := range started {
		files, err := os.ReadFile(f) //nolint:gosec // devvm's run folder
		if err != nil {
			continue
		}
		for file := range strings.SplitSeq(string(files), ",") {
			if rel, err := filepath.Rel(d.Assets(), file); err == nil && rel != "." && filepath.IsLocal(rel) {
				inUse[strings.Split(filepath.ToSlash(rel), "/")[0]] = true
			}
		}
	}
	return inUse
}

// EventLog is a VM's log of its proxy's decisions (JSON lines).
func EventLog(d paths.Dirs, vm string) string { return filepath.Join(d.Logs(), vm+".jsonl") }

// ProxyLog is a VM's proxy's own output.
func ProxyLog(d paths.Dirs, vm string) string { return filepath.Join(d.Logs(), vm+".proxy.log") }

// PID returns the pid of the VM's running proxy: the process its pid file
// names, if that still runs as the VM's proxy.
func (p *Proxy) PID(vm string) (int, bool) {
	b, err := os.ReadFile(p.run(vm, ".pid"))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || !alive(pid) {
		return 0, false
	}
	args, err := p.Args(pid)
	if err != nil || len(args) < 2 || args[1] != "_proxy" {
		return 0, false
	}
	if i := slices.Index(args, "-vm"); i < 0 || i+1 >= len(args) || args[i+1] != vm {
		return 0, false
	}
	return pid, true
}

func alive(pid int) bool { return signal(pid, syscall.Signal(0)) == nil }

func signal(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}

// Stop stops the VM's proxy, if it runs: SIGTERM, then after two seconds
// SIGKILL, waiting up to two more for it to end. It reports whether no
// proxy of the VM's runs then.
func (p *Proxy) Stop(vm string) bool {
	pid, ok := p.PID(vm)
	if !ok {
		return true
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		_ = p.Signal(pid, sig)
		for range 20 {
			if !alive(pid) {
				return true
			}
			time.Sleep(p.Tick / 5)
		}
		// Still the VM's proxy, not a process given its pid since.
		if _, ok := p.PID(vm); !ok {
			return true
		}
	}
	return false
}

// Forget removes what a VM of this name left behind when it was deleted
// some other way: its proxy, its own rules and its files in the run folder.
func (p *Proxy) Forget(vm string) error {
	p.Stop(vm)
	if err := os.Remove(VMPolicyFile(p.Dirs, vm)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	files, err := filepath.Glob(filepath.Join(p.Dirs.Run(), vm+".*"))
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Prepare readies a VM with network policy to start on b, or a running one
// to go on: devvm's CA, the VM's secret files, its ports, and its proxy
// running. It returns the confinement its backend starts it with. The proxy
// follows the host process that runs the VM (b's Lifeline): a VM started
// again gets a new proxy. On a backend with a decider, the proxy is the
// decider too, with a token new at each start.
func (p *Proxy) Prepare(ctx context.Context, b backend.Backend, st *state.VM, running bool) (backend.Confinement, error) {
	vm := st.Name()
	lifeline := b.Lifeline(ctx, vm)
	none := backend.Confinement{}
	// The CA lasts 30 days: a new one is made when it has under a week left
	// (VMs trust it from their next start). Kept out of backups: a copy of
	// its key elsewhere could impersonate any site to VMs.
	ca := p.Dirs.CA()
	if err := os.MkdirAll(ca, 0o700); err != nil {
		return none, err
	}
	if err := os.Chmod(ca, 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
		return none, err
	}
	if err := p.InitCA(ca); err != nil {
		return none, msg.ProxyCAFailed(ca)
	}
	if p.OS == "darwin" {
		_ = p.ExcludeFromBackup(ctx, ca)
	}
	key, err := Key(st)
	if err != nil {
		return none, err
	}
	salt, err := Salt(st)
	if err != nil {
		return none, err
	}
	if err := p.Secrets.WriteFiles(ctx, st, salt); err != nil {
		return none, err
	}
	// A stopped VM's proxy follows the hostagent it started with: a new
	// start gets a new one. One whose allowlist files changed (tools added
	// or removed), from another devvm binary (an upgrade), or holding a CA
	// since replaced (which mints certificates the VM no longer trusts
	// after its next setup) starts again too, on the same port.
	policies, err := p.PolicyFiles(vm, st.Get(state.KeyImage))
	if err != nil {
		return none, err
	}
	files := strings.Join(policies, ",")
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
	if !running || p.stale(vm, files, bin, caSum, cas.Bundle(), upstream) {
		// Never the old one kept: it would go on with what it started with.
		if !p.Stop(vm) {
			return none, msg.ProxyStopFailed(vm)
		}
	}
	sp, err := p.SSHPort(ctx, st, running)
	if err != nil {
		return none, err
	}
	_, up := p.PID(vm)
	pp, err := p.port(ctx, st, KeyProxyPort, up, running)
	if err != nil {
		return none, err
	}
	conf := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: pp, ProxyUser: ProxyUser, ProxyPassword: key, Env: Env, SSHPort: sp,
	}
	if backend.UsesDecider(b) {
		dp, err := p.port(ctx, st, KeyDeciderPort, up, running)
		if err != nil {
			return none, err
		}
		if conf.DeciderToken, err = deciderToken(st, running); err != nil {
			return none, err
		}
		conf.Decider = "127.0.0.1:" + strconv.Itoa(dp)
	}
	// The logs hold each VM's requests: the user's alone.
	for _, d := range []string{p.Dirs.Run(), p.Dirs.Logs()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return none, err
		}
		if err := os.Chmod(d, 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return none, err
		}
	}
	if up {
		return conf, nil
	}
	if err := p.writeUpstream(vm, cas.Bundle(), upstream); err != nil {
		return none, err
	}
	return conf, p.start(st, b, conf, files, bin, caSum, lifeline)
}

// stale reports whether the VM's proxy started with other settings than
// these: its allowlist files (tools added or removed), devvm's binary (an
// upgrade), devvm's CA (replaced, so it mints certificates the VM no
// longer trusts after its next setup), the user's CAs, or the company
// proxy it goes through.
func (p *Proxy) stale(vm, files, bin, caSum string, cas, upstream []byte) bool {
	oldCAs, _ := os.ReadFile(p.run(vm, ".cas"))
	oldUp, _ := os.ReadFile(p.run(vm, ".upstream"))
	return read(p.run(vm, ".policy")) != files || read(p.run(vm, ".bin")) != bin || read(p.run(vm, ".ca")) != caSum ||
		!bytes.Equal(oldCAs, cas) || !bytes.Equal(oldUp, upstream)
}

// SSHPort returns the host port the VM's ssh is forwarded from, with or
// without network policy, as its state records it: devvm chooses it,
// rather than the backend at its start, so every VM's port forwarder
// avoids it (UsedPorts) before the VM's backend binds it. A stopped VM
// gets a new one when its port is now another program's, on any address,
// or one of its own forwards, which its backend would refuse.
func (p *Proxy) SSHPort(ctx context.Context, st *state.VM, running bool) (int, error) {
	sp, _ := strconv.Atoi(st.Get(KeySSHPort))
	if sp != 0 && (running || !forwarded(st)[sp] && !p.Listening(ctx, sp)) {
		return sp, nil
	}
	return p.choosePort(ctx, st, KeySSHPort)
}

// choosePort chooses a free port (freePort) and records it as the VM's
// port under key, holding devvm's ports lock throughout, so two devvm runs
// making or starting VMs at once never choose the same one.
func (p *Proxy) choosePort(ctx context.Context, st *state.VM, key string) (int, error) {
	if err := os.MkdirAll(p.Dirs.Run(), 0o700); err != nil {
		return 0, err
	}
	unlock, err := platform.LockFile(ctx, filepath.Join(p.Dirs.Run(), "ports.lock"))
	if err != nil {
		return 0, err
	}
	defer unlock()
	n, err := p.freePort(ctx)
	if err != nil {
		return 0, err
	}
	return n, st.Set(key, strconv.Itoa(n))
}

// port returns the VM's port under key: a new free one when it has none,
// or when its proxy doesn't run and another program took it, unless the VM
// runs: its backend holds the port it started with.
func (p *Proxy) port(ctx context.Context, st *state.VM, key string, up, running bool) (int, error) {
	n, _ := strconv.Atoi(st.Get(key))
	if up || n != 0 && !p.Listening(ctx, n) {
		return n, nil
	}
	if running && n != 0 {
		return 0, msg.ProxyPortTaken(strconv.Itoa(n), st.Name())
	}
	return p.choosePort(ctx, st, key)
}

// RefreshSecrets writes again the secret files of the VMs named (none:
// every VM) whose proxy has them, so a token saved, deleted, bound or
// unbound applies at once: a proxy reads its files again when they change,
// and looks its values up afresh. Its logins' bindings stay as its start
// wrote them.
func (p *Proxy) RefreshSecrets(ctx context.Context, vms ...string) error {
	if len(vms) == 0 {
		files, err := filepath.Glob(secrets.BindingsFile(p.Dirs, "*"))
		if err != nil {
			return err
		}
		for _, f := range files {
			vms = append(vms, strings.TrimSuffix(filepath.Base(f), ".secrets.json"))
		}
	}
	for _, vm := range vms {
		if _, err := os.Stat(secrets.BindingsFile(p.Dirs, vm)); err != nil {
			continue
		}
		st, err := state.ReadVM(p.Dirs, vm)
		if err != nil || !st.Exists() || st.Get(KeyProxySalt) == "" {
			continue // not a VM devvm still has
		}
		if err := p.Secrets.RefreshFiles(ctx, st, st.Get(KeyProxySalt)); err != nil {
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
		if err := writeFile(path, f.data); err != nil {
			return err
		}
	}
	return nil
}

// start starts the VM's proxy as conf has it, and waits for it to listen.
func (p *Proxy) start(st *state.VM, b backend.Backend, conf backend.Confinement, files, bin, caSum, lifeline string) error {
	vm := st.Name()
	// The password and the decider's token go in files: a process's
	// arguments are visible to other users.
	auth := p.run(vm, ".auth")
	if err := writeFile(auth, []byte(conf.ProxyPassword+"\n")); err != nil {
		return err
	}
	var decider []string
	if conf.Decider != "" {
		u, err := url.Parse(b.ProxyURL(conf))
		if err != nil || u.Host == "" {
			return fmt.Errorf("%s: no proxy address in the VM", vm)
		}
		tok := p.run(vm, ".decider")
		if err := writeFile(tok, []byte(conf.DeciderToken+"\n")); err != nil {
			return err
		}
		decider = []string{"-decider", conf.Decider, "-decider-to", u.Host, "-decider-token-file", tok}
	}
	// A VM that reaches the proxy through a socket of the host's: in a
	// folder of its own, the user's alone.
	if sock := backend.ProxySocketOf(b, vm); sock != "" {
		if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Dir(sock), 0o700); err != nil { //nolint:gosec // a folder: owner-only with search
			return err
		}
		decider = append(decider, "-listen-unix", sock)
	}
	logPath := ProxyLog(p.Dirs, vm)
	out, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // devvm's own log
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	fi, err := out.Stat()
	if err != nil {
		return err
	}
	before := fi.Size()
	ca := p.Dirs.CA()
	addr := "127.0.0.1:" + strconv.Itoa(conf.ProxyPort)
	args := append([]string{
		"_proxy", "-listen", addr, "-vm", vm, "-policy", files, "-log", EventLog(p.Dirs, vm), "-proxy-log", logPath, "-lifeline", lifeline,
		"-ca", filepath.Join(ca, "ca.pem"), "-cakey", filepath.Join(ca, "ca.key"),
		"-secrets", secrets.BindingsFile(p.Dirs, vm), "-secret-cmd", p.Self, "-auth-file", auth,
	}, decider...)
	for _, f := range [][2]string{{".cas", "-upstream-cas"}, {".upstream", "-upstream"}} {
		if _, err := os.Stat(p.run(vm, f[0])); err == nil {
			args = append(args, f[1], p.run(vm, f[0]))
		}
	}
	pid, err := p.Start(p.Self, args, out)
	if err != nil {
		return err
	}
	for _, f := range [][2]string{{".pid", strconv.Itoa(pid)}, {".policy", files}, {".bin", bin}, {".ca", caSum}} {
		if err := writeFile(p.run(vm, f[0]), []byte(f[1]+"\n")); err != nil {
			return err
		}
	}
	// Started means this proxy says it listens on the port, and still runs:
	// another program that took the port first would otherwise get the
	// VM's traffic.
	want := []byte("listening on " + addr + " for ")
	for range 10 {
		if logged(logPath, before, want) {
			if _, ok := p.PID(vm); ok {
				return nil
			}
		}
		time.Sleep(p.Tick)
	}
	return msg.ProxyStartFailed(logPath, vm)
}

// logged reports whether a log has want after its first before bytes. A
// log shorter than that was rotated (copied to <log>.1, then emptied), so
// the rest of <log>.1 and the whole log count.
func logged(path string, before int64, want []byte) bool {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own log
	if err != nil {
		return false
	}
	if int64(len(b)) >= before {
		return bytes.Contains(b[before:], want)
	}
	if bytes.Contains(b, want) {
		return true
	}
	old, err := os.ReadFile(path + ".1") //nolint:gosec // devvm's own log
	return err == nil && int64(len(old)) >= before && bytes.Contains(old[before:], want)
}

// freePort returns a loopback port from 20000 to 59999 no program has
// (Listening) and no VM has.
func (p *Proxy) freePort(ctx context.Context) (int, error) {
	used, err := UsedPorts(p.Dirs, "")
	if err != nil {
		return 0, err
	}
	for range 1000 {
		port := 20000 + rand.IntN(40000) //nolint:gosec // a port, not a secret
		if !used[port] && !p.Listening(ctx, port) {
			return port, nil
		}
	}
	return 0, errors.New("no free port on 127.0.0.1")
}

// UsedPorts returns the ssh, proxy, decider and desktop ports VMs have, and the
// ports they list to forward, but self's (a VM's name, or ""): what a new
// one avoids, and what a VM's port forwarder never takes (smolvm's,
// Docker's), self being that VM.
func UsedPorts(d paths.Dirs, self string) (map[int]bool, error) {
	ents, err := os.ReadDir(d.VMs())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	used := map[int]bool{}
	for _, e := range ents {
		if !e.Type().IsRegular() || strings.Contains(e.Name(), ".") {
			continue
		}
		st, err := state.ReadVM(d, e.Name())
		if err != nil {
			continue // not a VM's state
		}
		for _, k := range []string{KeySSHPort, KeyProxyPort, KeyDeciderPort, KeyDesktopPort} {
			if n, err := strconv.Atoi(st.Get(k)); err == nil {
				used[n] = true
			}
		}
		if e.Name() == self {
			continue
		}
		for n := range forwarded(st) {
			used[n] = true
		}
	}
	return used, nil
}

// keyPorts is the key of a VM's state that lists the ports it forwards to
// the same host port (vm's KeyPorts).
const keyPorts = "ports"

// forwarded returns the host ports a VM forwards, as its state lists them
// (5173,8000-8010); none for all or none, which name no port.
func forwarded(st *state.VM) map[int]bool {
	out := map[int]bool{}
	for r := range strings.SplitSeq(st.Get(keyPorts), ",") {
		from, to, ok := strings.Cut(strings.TrimSpace(r), "-")
		if !ok {
			to = from
		}
		a, err1 := strconv.Atoi(from)
		b, err2 := strconv.Atoi(to)
		if err1 != nil || err2 != nil || a < 1 || b > 65535 || a > b {
			continue
		}
		for n := a; n <= b; n++ {
			out[n] = true
		}
	}
	return out
}

// listening reports whether a program has a host loopback port: one that
// has it on any address (platform.PortInUse), even if it never accepts,
// or one that accepts connections on it.
func listening(ctx context.Context, port int) bool {
	if platform.PortInUse(ctx, port) {
		return true
	}
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(ctx, "tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func read(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own run folder
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeFile writes a file whole and owner-only, through a temporary file
// renamed over it.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
