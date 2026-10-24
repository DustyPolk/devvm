package netpolicy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
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
// background, following the host process that runs the VM. NewProxy sets
// every field; tests change them.
type Proxy struct {
	Dirs    paths.Dirs
	Root    string // devvm's own files: layers/, tools/
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
}

// NewProxy returns the proxies of devvm's folders, run by self (devvm's
// binary), with its files in root.
func NewProxy(d paths.Dirs, root, self string, store *secrets.Store) *Proxy {
	return &Proxy{
		Dirs: d, Root: root, Self: self, OS: runtime.GOOS, Secrets: store,
		Start: platform.StartDetached, Args: platform.ProcessArgs, ExcludeFromBackup: platform.ExcludeFromBackup,
		InitCA: func(dir string) error {
			return proxy.Main([]string{"-init-ca", dir}, proxy.IO{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: io.Discard})
		},
		Listening: listening, Tick: 500 * time.Millisecond,
	}
}

func (p *Proxy) run(vm, ext string) string { return filepath.Join(p.Dirs.Run(), vm+".proxy"+ext) }

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

// Prepare readies a VM with network policy to start, or a running one to
// go on: devvm's CA, the VM's secret files, its ports, and its proxy
// running. It returns the confinement its backend starts it with.
// lifeline is the pid file of the host process that runs the VM, which the
// proxy follows: a VM started again gets a new proxy.
func (p *Proxy) Prepare(ctx context.Context, st *state.VM, lifeline string, running bool) (backend.Confinement, error) {
	vm := st.Name()
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
	files := strings.Join(PolicyFiles(p.Dirs, p.Root, vm, st.Get(state.KeyImage)), ",")
	bin, _ := image.SHA256File(p.Self)
	caSum, _ := image.SHA256File(filepath.Join(ca, "ca.pem"))
	if !running || read(p.run(vm, ".policy")) != files || read(p.run(vm, ".bin")) != bin || read(p.run(vm, ".ca")) != caSum {
		p.Stop(vm)
	}
	sp, _ := strconv.Atoi(st.Get(KeySSHPort))
	if sp == 0 || !running && p.Listening(sp) {
		if sp, err = p.freePort(); err != nil {
			return none, err
		}
		if err := st.Set(KeySSHPort, strconv.Itoa(sp)); err != nil {
			return none, err
		}
	}
	pp, _ := strconv.Atoi(st.Get(KeyProxyPort))
	_, up := p.PID(vm)
	if !up && (pp == 0 || p.Listening(pp)) {
		if running && pp != 0 {
			return none, msg.ProxyPortTaken(strconv.Itoa(pp), vm)
		}
		if pp, err = p.freePort(); err != nil {
			return none, err
		}
		if err := st.Set(KeyProxyPort, strconv.Itoa(pp)); err != nil {
			return none, err
		}
	}
	conf := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: pp, ProxyUser: ProxyUser, ProxyPassword: key, Env: Env, SSHPort: sp,
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
	return conf, p.start(st, key, files, bin, caSum, lifeline, pp)
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

// start starts the VM's proxy on port pp and waits for it to listen.
func (p *Proxy) start(st *state.VM, key, files, bin, caSum, lifeline string, pp int) error {
	vm := st.Name()
	// The password goes in a file: a process's arguments are visible to
	// other users.
	auth := p.run(vm, ".auth")
	if err := writeFile(auth, []byte(key+"\n")); err != nil {
		return err
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
	addr := "127.0.0.1:" + strconv.Itoa(pp)
	args := []string{
		"_proxy", "-listen", addr, "-vm", vm, "-policy", files, "-log", EventLog(p.Dirs, vm), "-lifeline", lifeline,
		"-ca", filepath.Join(ca, "ca.pem"), "-cakey", filepath.Join(ca, "ca.key"),
		"-secrets", secrets.BindingsFile(p.Dirs, vm), "-secret-cmd", p.Self, "-auth-file", auth,
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

// usedPorts returns the ssh and proxy ports VMs have.
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
		for _, k := range []string{KeySSHPort, KeyProxyPort} {
			if n, err := strconv.Atoi(st.Get(k)); err == nil {
				used[n] = true
			}
		}
	}
	return used, nil
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
