package lima

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
)

// A build behind a company proxy (the host's HTTPS_PROXY or HTTP_PROXY)
// reaches it through a proxy of devvm's on the host for that build alone
// (backend.ProxyStarter), which has the company proxy's address and
// password in its -upstream file. The build VM gets only its own proxy's
// address and a password made for the build, never the company proxy's,
// so the code a build runs (an image's own, with --from) can't read the
// company proxy's credentials or write them into an image. On macOS the
// build VM's sandbox allows that one port of the host's loopback.
//
// Nothing of it is written into the build VM either: Lima's
// propagateProxyEnv would put it in the guest's /etc/environment, and so
// in the image. Each build script gets the variables exported at its
// start instead, fed on its input with the script (in no process's
// arguments and no file), and sudo keeps them while the build runs.

// buildProxyHost is the host's loopback as a build VM reaches it: the
// host's address on Lima's user-mode network (host.lima.internal), with
// vz and with QEMU, whose network a build keeps whole (no restrict=on).
// An address, not the name, so a chroot with its own /etc/hosts reaches
// it too.
const buildProxyHost = "192.168.5.2"

// sudoKeepProxy is the sudoers line that keeps a build's proxy variables
// for sudo, in the build VM while its scripts run.
const sudoKeepProxy = `Defaults env_keep += "http_proxy https_proxy no_proxy HTTP_PROXY HTTPS_PROXY NO_PROXY"`

// buildUpstream is the company proxy a build on n goes through: the
// host's proxy settings, as getenv has them, and the -upstream file its
// proxy reads; none (nil) without the internet or a proxy on the host. A
// setting the proxy can't go through is its message.
func buildUpstream(getenv func(string) string, n backend.Net) (backend.HostProxy, []byte, error) {
	if n != backend.NetInternet {
		return backend.HostProxy{}, nil, nil
	}
	host := backend.ReadHostProxy(getenv)
	up, err := host.Upstream()
	if err != nil {
		return backend.HostProxy{}, nil, err
	}
	return host, up, nil
}

// buildProxy starts the proxy of a build VM (vm) on n: with the company
// proxy that buildUpstream finds, devvm's proxy for the build alone,
// following the VM's hostagent. It returns the confinement the VM starts
// with (on macOS its sandbox allows the proxy's port), what the build's
// scripts get (the proxy's address with the build's password for each
// variable the host sets, and the host's NO_PROXY), and done, which stops
// the proxy and with ok deletes its folder (a failed build keeps its
// logs). Without a company proxy, nothing is started and the scripts get
// nothing.
func buildProxy(ctx context.Context, l *Lima, vm string, n backend.Net) (backend.Confinement, backend.HostProxy, func(ok bool), error) {
	c := backend.Confinement{Net: n}
	none := func(bool) {}
	host, up, err := buildUpstream(l.Getenv, n)
	if err != nil || up == nil {
		return c, backend.HostProxy{}, none, err
	}
	if ctx.Err() != nil {
		return c, backend.HostProxy{}, none, ctx.Err()
	}
	dir := filepath.Join(l.Dirs.Run(), "builds", vm)
	starter := backend.ProxyStarter{Self: l.Self, Spawn: l.Spawn, Tick: l.Tick}
	// The build VM reaches internal addresses directly anyway, so names this
	// machine can't resolve go on to the company proxy.
	p, err := starter.Start(vm, dir, l.haPIDFile(vm), up, "-upstream-unresolved")
	if err != nil {
		return c, backend.HostProxy{}, none, err
	}
	remove := l.AddCleanup(func(context.Context) { p.Stop() })
	done := func(ok bool) {
		p.Stop()
		remove()
		if ok {
			_ = os.RemoveAll(dir)
		}
	}
	c.ProxyPort = p.Port
	u := p.URL(buildProxyHost)
	hp := backend.HostProxy{NoProxy: host.NoProxy}
	if host.HTTPS != "" {
		hp.HTTPS = u
	}
	if host.HTTP != "" {
		hp.HTTP = u
	}
	return c, hp, done, nil
}

// proxyExports are shell lines exporting h's variables, for the start of a
// build script: none without a proxy.
func proxyExports(h backend.HostProxy) []byte {
	env := h.Env()
	if len(env) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("export")
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		b.WriteString(" " + k + "=" + shq(v))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// withProxy is a build script with h's variables exported at its start.
func withProxy(h backend.HostProxy, s backend.Script) backend.Script {
	if pre := proxyExports(h); pre != nil && len(s.Data) > 0 {
		s.Data = append(pre, s.Data...)
	}
	return s
}

// sudoScripts are the root scripts that keep h's variables for sudo while
// a build's scripts run, and take that out again before it stops: none
// without a proxy.
func sudoScripts(h backend.HostProxy) (on, off backend.Script) {
	if h.None() {
		return backend.Script{}, backend.Script{}
	}
	f := "/etc/sudoers.d/devvm-build-proxy"
	return backend.Script{Name: "sudo-proxy", Data: []byte("printf '%s\\n' " + shq(sudoKeepProxy) + " > " + f + " && chmod 440 " + f + "\n")},
		backend.Script{Name: "sudo-proxy", Data: []byte("rm -f " + f + "\n")}
}
