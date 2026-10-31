// Package netpolicy runs network policy for VMs: the choice for a new VM,
// the VM's allowlist files, its proxy on the host (devvm _proxy), the
// confinement its backend starts it with, and the self-check after each
// start that it can't reach around the proxy.
package netpolicy

import (
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
)

// Keys of a VM's state.
const (
	KeyNet       = "net"        // network policy: on or off
	KeyCheck     = "net_check"  // the last self-check's result: ok or unenforced
	KeyProxyKey  = "proxy_key"  // the VM's proxy password
	KeyProxySalt = "proxy_salt" // what ends the VM's placeholders
	KeySSHPort   = "ssh_port"   // the host port of the VM's ssh (Proxy.SSHPort)
	KeyProxyPort = "proxy_port" // the host loopback port its proxy listens on
	// On a backend with a decider (backend.Decider): the host loopback port
	// the proxy decides on, and the token its backend proves itself with,
	// new at each start.
	KeyDeciderPort  = "decider_port"
	KeyDeciderToken = "decider_token"
)

// ProxyUser is the user in a VM's proxy URL; the VM's key is the password.
const ProxyUser = "devvm"

// On reports whether a VM has network policy.
func On(st *state.VM) bool { return st.Get(KeyNet) == "on" }

// Choose returns whether a new VM gets network policy: what the flags say,
// else the answer in a terminal (yes by default), else on.
func (p *Proxy) Choose(on, off bool, vm string) bool {
	switch {
	case on:
		return true
	case off:
		return false
	case !p.Terminal():
		return true
	}
	p.Note(msg.NetAbout(vm))
	return p.Confirm(msg.NetAsk())
}

// Key returns the VM's own random key, made the first time it's needed. It's
// the password for the VM's proxy, so other VMs and programs on the host
// that reach its port can't use it.
func Key(st *state.VM) (string, error) { return random(st, KeyProxyKey) }

// Salt returns what ends the VM's placeholders: random, like the key, and
// not the key itself, so a placeholder the VM sends elsewhere doesn't give
// its proxy's password away.
func Salt(st *state.VM) (string, error) { return random(st, KeyProxySalt) }

// deciderToken returns the token the VM's backend proves itself to its
// proxy's decider with, a new one unless the VM runs (its backend holds the
// one it started with): 32 random bytes, as 64 hex digits.
func deciderToken(st *state.VM, running bool) (string, error) {
	if v := st.Get(KeyDeciderToken); v != "" && running {
		return v, nil
	}
	return newRandom(st, KeyDeciderToken, 32)
}

func random(st *state.VM, key string) (string, error) {
	if v := st.Get(key); v != "" {
		return v, nil
	}
	return newRandom(st, key, 16)
}

// newRandom sets key to n new random bytes, as hex.
func newRandom(st *state.VM, key string, n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	v := hex.EncodeToString(b)
	return v, st.Set(key, v)
}

// PolicyFiles returns a VM's policy files, in order: base's and each of its
// image's parts' (a shared tool's from devvm's shared-tools/, a built-in
// layer's from devvm's layers/, another layer's from the user's layers),
// the user's own for every VM, the VM's own, and
// the hosts its secrets imply. devvm's own are in assets, the folder
// they're written out in. A missing file has no rules.
func PolicyFiles(d paths.Dirs, assets, vm, img string) []string {
	files := []string{filepath.Join(assets, "layers", "base.policy")}
	for p := range strings.SplitSeq(img, "+") {
		switch {
		case p == "" || p == "base":
		case image.IsTool(p):
			files = append(files, filepath.Join(assets, "shared-tools", p+".policy"))
		case image.IsBuiltin(p):
			files = append(files, filepath.Join(assets, "layers", p+".policy"))
		default:
			files = append(files, filepath.Join(d.Layers(), p+".policy"))
		}
	}
	return append(files, d.Policy(), VMPolicyFile(d, vm), secrets.PolicyFile(d, vm))
}

// VMPolicyFile holds the rules of one VM's own.
func VMPolicyFile(d paths.Dirs, vm string) string { return d.VMState(vm) + ".policy" }

// Env is the guest environment a VM with network policy gets besides the
// proxy's address: no proxy for the VM itself, and Node told to use the
// proxy variables. Tools find the proxy's CA as every VM finds the user's
// (trust.Env).
var Env = map[string]string{
	"NO_PROXY":           "localhost,127.0.0.1,::1",
	"no_proxy":           "localhost,127.0.0.1,::1",
	"NODE_USE_ENV_PROXY": "1",
}
