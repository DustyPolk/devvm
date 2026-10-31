package lima

import (
	"net"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/proxy"
)

// A build's commands reach the internet through the host's proxy, when
// it has one, but nothing of it is written into the build VM: Lima's
// propagateProxyEnv would put it in the guest's /etc/environment, and so in
// the image, password and all. Each build script gets the variables
// exported at its start instead, fed on its input with the script (in no
// process's arguments and no file), and sudo keeps them while the build
// runs (buildSudo).

// sudoKeepProxy is the sudoers line that keeps a build's proxy variables
// for sudo, in the build VM while its scripts run.
const sudoKeepProxy = `Defaults env_keep += "http_proxy https_proxy no_proxy HTTP_PROXY HTTPS_PROXY NO_PROXY"`

// buildProxy is the host's proxy, as getenv has it, as a build VM reaches
// it: one on the host's loopback through Lima's address for the host, as
// Lima's own propagateProxyEnv does.
func buildProxy(getenv func(string) string) backend.HostProxy {
	h := backend.ReadHostProxy(getenv)
	h.HTTPS, h.HTTP = guestProxyURL(h.HTTPS), guestProxyURL(h.HTTP)
	return h
}

// guestProxyURL is a proxy URL with a loopback host replaced by
// host.lima.internal; any other, or one devvm can't read, as it is.
func guestProxyURL(raw string) string {
	u, err := proxy.ParseUpstream(raw)
	if err != nil {
		return raw
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		if p := u.Port(); p != "" {
			u.Host = net.JoinHostPort("host.lima.internal", p)
		} else {
			u.Host = "host.lima.internal"
		}
		return u.String()
	}
	return raw
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
