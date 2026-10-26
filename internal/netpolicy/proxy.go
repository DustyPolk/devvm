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
	// Listening reports whether something listens on a host loopback port.
	Listening func(port int) bool
	// Tick is how often a start looks for the proxy to listen, ten times.
	Tick time.Duration
	// What it prints, and asks: [y/N] and [Y/n]; Terminal reports whether
	// it can ask.
	Note, Warn   func(msg.M)
	Ask, Confirm func(msg.M) bool
	Terminal     func() bool
}

// UI is where the proxies print and ask.
type UI interface {
	Note(msg.M)
	Warn(msg.M)
	Ask(msg.M) bool
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
		Listening: listening, Tick: 500 * time.Millisecond,
		Note: u.Note, Warn: u.Warn, Ask: u.Ask, Confirm: func(m msg.M) bool { return u.Confirm(m) },
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

// Stop stops the VM's proxy, if it runs, waiting up to two seconds for it
// to end.
func (p *Proxy) Stop(vm string) {
	pid, ok := p.PID(vm)
	if !ok {
		return
	}
	_ = signal(pid, syscall.SIGTERM)
	for range 10 {
		if !alive(pid) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
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
	if !running || read(p.run(vm, ".policy")) != files || read(p.run(vm, ".bin")) != bin || read(p.run(vm, ".ca")) != caSum {
		p.Stop(vm)
	}
	sp, _ := strconv.Atoi(st.Get(KeySSHPort))
	// Never one of the VM's own forwards: its backend would refuse it.
	if sp == 0 || !running && (p.Listening(sp) || forwarded(st)[sp]) {
		if sp, err = p.freePort(); err != nil {
			return none, err
		}
		if err := st.Set(KeySSHPort, strconv.Itoa(sp)); err != nil {
			return none, err
		}
	}
	_, up := p.PID(vm)
	pp, err := p.port(st, KeyProxyPort, up, running)
	if err != nil {
		return none, err
	}
	conf := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: pp, ProxyUser: ProxyUser, ProxyPassword: key, Env: Env, SSHPort: sp,
	}
	if backend.UsesDecider(b) {
		dp, err := p.port(st, KeyDeciderPort, up, running)
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
	return conf, p.start(st, b, conf, files, bin, caSum, lifeline)
}

// port returns the VM's port under key: a new free one when it has none,
// or when its proxy doesn't run and another program took it, unless the VM
// runs: its backend holds the port it started with.
func (p *Proxy) port(st *state.VM, key string, up, running bool) (int, error) {
	n, _ := strconv.Atoi(st.Get(key))
	if up || n != 0 && !p.Listening(n) {
		return n, nil
	}
	if running && n != 0 {
		return 0, msg.ProxyPortTaken(strconv.Itoa(n), st.Name())
	}
	n, err := p.freePort()
	if err != nil {
		return 0, err
	}
	return n, st.Set(key, strconv.Itoa(n))
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
		"_proxy", "-listen", addr, "-vm", vm, "-policy", files, "-log", EventLog(p.Dirs, vm), "-lifeline", lifeline,
		"-ca", filepath.Join(ca, "ca.pem"), "-cakey", filepath.Join(ca, "ca.key"),
		"-secrets", secrets.BindingsFile(p.Dirs, vm), "-secret-cmd", p.Self, "-auth-file", auth,
	}, decider...)
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

// logged reports whether a log has want after its first before bytes.
func logged(path string, before int64, want []byte) bool {
	b, err := os.ReadFile(path) //nolint:gosec // devvm's own log
	if err != nil || int64(len(b)) < before {
		return false
	}
	return bytes.Contains(b[before:], want)
}

// freePort returns a loopback port from 20000 to 59999 nothing listens on
// and no VM has.
func (p *Proxy) freePort() (int, error) {
	used, err := usedPorts(p.Dirs)
	if err != nil {
		return 0, err
	}
	for range 1000 {
		port := 20000 + rand.IntN(40000) //nolint:gosec // a port, not a secret
		if !used[port] && !p.Listening(port) {
			return port, nil
		}
	}
	return 0, errors.New("no free port on 127.0.0.1")
}

// usedPorts returns the ssh, proxy and decider ports VMs have.
func usedPorts(d paths.Dirs) (map[int]bool, error) {
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
		for _, k := range []string{KeySSHPort, KeyProxyPort, KeyDeciderPort} {
			if n, err := strconv.Atoi(st.Get(k)); err == nil {
				used[n] = true
			}
		}
		for n := range forwarded(st) {
			used[n] = true
		}
	}
	return used, nil
}

// The keys of a VM's state that list the ports it forwards to the same
// host port: wanted, and set in its backend (vm's KeyPorts and
// KeyPortsSet).
const (
	keyPorts    = "ports"
	keyPortsSet = "ports_set"
)

// forwarded returns the host ports a VM forwards, as its state lists them
// (5173,8000-8010); none for all or none, which name no port.
func forwarded(st *state.VM) map[int]bool {
	out := map[int]bool{}
	for _, k := range []string{keyPorts, keyPortsSet} {
		for r := range strings.SplitSeq(st.Get(k), ",") {
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
	}
	return out
}

// listening reports whether something accepts connections on a host
// loopback port.
func listening(port int) bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second) //nolint:noctx // a moment's dial
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
