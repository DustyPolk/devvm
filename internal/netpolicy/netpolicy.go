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

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
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
	// The host loopback port devvm desktop serves the VM's desktop on,
	// which no VM's ssh, proxy or forwards take.
	KeyDesktopPort = "desktop_port"
)

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

// Key returns the VM's own random key, made the first time it's needed
// (backend.NewProxyPassword). It's the password for the VM's proxy, so
// other VMs and programs on the host that reach its port can't use it.
func Key(st *state.VM) (string, error) {
	if v := st.Get(KeyProxyKey); v != "" {
		return v, nil
	}
	v, err := backend.NewProxyPassword()
	if err != nil {
		return "", err
	}
	return v, st.Set(KeyProxyKey, v)
}

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

// Policies returns the policies of the VM vm, of image img with these
// shared tools, in the order its proxy reads them. builtin is devvm's own,
// as paths in its files (assets.FS), which the proxy reads from the
// binary: base's, then each shared tool's in shared-tools/, then each
// built-in layer's in layers/. files is the user's: each of the image's
// own layers' policy file, next to its script, the user's own for every
// VM, the VM's own, and the hosts its secrets imply. A missing file has no
// rules.
func Policies(d paths.Dirs, vm, img string, tools []string) (builtin, files []string) {
	builtin = []string{"layers/base.policy"}
	for _, t := range tools {
		// Only a shared tool's name names a file of devvm's.
		if image.IsTool(t) {
			builtin = append(builtin, "shared-tools/"+t+".policy")
		}
	}
	for p := range strings.SplitSeq(img, "+") {
		switch {
		case p == "" || p == "base" || image.IsTool(p):
		case image.IsBuiltin(p):
			builtin = append(builtin, "layers/"+p+".policy")
		case image.LayerName(p):
			files = append(files, filepath.Join(d.Layers(), p+".policy"))
		}
		// A part that's no layer's name (../x) names no file: its path
		// would leave the user's layers, so it adds no rules.
	}
	return builtin, append(files, d.Policy(), d.VM(vm).Policy(), d.VM(vm).SecretsPolicy())
}

// Env is the guest environment a VM with network policy gets besides the
// proxy's address: no proxy for the VM itself, and Node told to use the
// proxy variables. Tools find the proxy's CA as every VM finds the user's
// (trust.Env).
var Env = map[string]string{
	"NO_PROXY":           NoProxy,
	"no_proxy":           NoProxy,
	"NODE_USE_ENV_PROXY": "1",
}

// NoProxy is what a VM with network policy reaches without its proxy: the
// VM itself. Its links aren't in it: an HTTP client that sends one to the
// proxy reaches it there (proxy's -links), so links change no VM's
// environment.
const NoProxy = "localhost,127.0.0.1,::1"
