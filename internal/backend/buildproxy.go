package backend

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
)

// A build that reaches the internet through a company proxy, or on Docker
// Desktop through any proxy, goes through a proxy of devvm's own on the
// host, one per build: devvm _proxy with any host on ports 80 and 443 but
// the host's own and internal addresses, through the company proxy (its
// -upstream file), and a password made for the build. The build gets only
// that proxy's address with that password, so the company proxy's
// credentials stay on the host, out of reach of the code a build runs,
// which is an image's own.

// buildProxyUser is the user in a build proxy's address.
const buildProxyUser = "devvm"

// ProxyStarter starts build proxies: devvm's own binary (Self), run by
// Spawn (daemon.Spawn: a process that can outlive devvm, stopped through
// the process itself), looked at every Tick while it starts, ten times at
// most.
type ProxyStarter struct {
	Self  string
	Spawn func(daemon.Cmd) (*daemon.Handle, error)
	Tick  time.Duration
}

// BuildProxy is one build's proxy: its port on the host's loopback and its
// password.
type BuildProxy struct {
	Port     int
	Password string
	proc     *daemon.Handle
	dir      string
}

// Start starts the proxy of a build (name), its files in dir, made again
// and the user's alone: its policy, its password, its log (proxy.log) and
// its event log (events.jsonl). It listens on a loopback port of its own,
// goes through the company proxy that upstream names (the -upstream file;
// nil for none) and ends when the process lifeline's pid file names does,
// or at Stop. extra are more of its arguments. It returns once the proxy
// says it listens, in its status file (status), and still runs.
func (s ProxyStarter) Start(name, dir, lifeline string, upstream []byte, extra ...string) (*BuildProxy, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	p := &BuildProxy{Password: hex.EncodeToString(b), dir: dir}
	auth, policy, logPath := filepath.Join(dir, "auth"), filepath.Join(dir, "policy"), filepath.Join(dir, "proxy.log")
	if err := state.WriteFile(auth, []byte(p.Password+"\n")); err != nil {
		return nil, err
	}
	if err := state.WriteFile(policy, []byte("# a build: any host, not this machine's own or internal addresses\n**\n")); err != nil {
		return nil, err
	}
	out, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // devvm's own log
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Close() }()
	args := []string{
		"_proxy", "-listen", "127.0.0.1:0", "-vm", name, "-policy", policy,
		"-log", filepath.Join(dir, "events.jsonl"), "-auth-file", auth, "-lifeline", lifeline,
	}
	args = append(args, extra...)
	if upstream != nil {
		f := filepath.Join(dir, "upstream")
		if err := state.WriteFile(f, upstream); err != nil {
			return nil, err
		}
		args = append(args, "-upstream", f)
	}
	status := filepath.Join(dir, "status")
	if p.proc, err = s.Spawn(daemon.Cmd{Path: s.Self, Args: args, Out: out, Status: status}); err != nil {
		return nil, err
	}
	said, ok := daemon.WaitReady(status, p.proc.PID(), p.proc.Running, s.Tick, 10)
	if ap, err := netip.ParseAddrPort(said); ok && err == nil && ap.Addr().IsLoopback() && ap.Port() != 0 {
		p.Port = int(ap.Port())
		return p, nil
	}
	p.Stop()
	return nil, msg.ProxyStartFailed(logPath, name)
}

// URL is the proxy's address as a build reaches it, on host, with its
// user and password.
func (p *BuildProxy) URL(host string) string {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(p.Port)), User: url.UserPassword(buildProxyUser, p.Password)}
	return u.String()
}

// Stop stops the proxy, through the process Start started (daemon.Handle),
// so the signal reaches it alone, never another process given its pid
// since: SIGTERM, then after two seconds SIGKILL. It deletes its password
// and the company proxy's file, leaving its logs. A nil one is none.
func (p *BuildProxy) Stop() {
	if p == nil {
		return
	}
	p.proc.Stop(2 * time.Second)
	for _, f := range []string{"auth", "upstream"} {
		_ = os.Remove(filepath.Join(p.dir, f))
	}
}
