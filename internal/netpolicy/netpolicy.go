// Package netpolicy runs network policy for VMs: the choice for a new VM,
// the VM's allowlist files, its proxy on the host (devvm _proxy), the
// confinement its backend starts it with, and the self-check after each
// start that it can't reach around the proxy.
package netpolicy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// Section is the name of the network policy section in a VM's state.
const Section = "net"

// Net is a VM's network policy section: whether it has network policy, as
// asked, and what devvm made for it.
type Net struct {
	On   bool    `json:"on"`
	Made NetMade `json:"made,omitzero"`
}

// NetMade is what devvm made for a VM's network policy.
type NetMade struct {
	Check     string `json:"check,omitempty"`      // the last self-check's result: CheckOK or CheckUnenforced
	ProxyKey  string `json:"proxy_key,omitempty"`  // the VM's proxy password (Key)
	ProxySalt string `json:"proxy_salt,omitempty"` // what ends the VM's placeholders (Salt)
}

// Of returns a VM's network policy section.
func Of(st *state.VM) (Net, error) { return state.Section[Net](st, Section) }

// Set sets a VM's network policy section, in st alone: inside st.Update.
func Set(st *state.VM, n Net) error { return state.SetSection(st, Section, n) }

// SetOn sets whether a VM has network policy, in st alone: inside
// st.Update.
func SetOn(st *state.VM, on bool) error {
	n, err := Of(st)
	if err != nil {
		return err
	}
	n.On = on
	return Set(st, n)
}

// On reports whether a VM has network policy. A section that can't be
// read (the folder's VM's is checked as it's found) has none.
func On(st *state.VM) bool {
	n, err := Of(st)
	return err == nil && n.On
}

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
func Key(ctx context.Context, st *state.VM) (string, error) {
	return made(ctx, st, func(m *NetMade) *string { return &m.ProxyKey }, backend.NewProxyPassword)
}

// Salt returns what ends the VM's placeholders: random, like the key, and
// not the key itself, so a placeholder the VM sends elsewhere doesn't give
// its proxy's password away.
func Salt(ctx context.Context, st *state.VM) (string, error) {
	return made(ctx, st, func(m *NetMade) *string { return &m.ProxySalt }, func() (string, error) {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		return hex.EncodeToString(b), nil
	})
}

// made returns the value of what devvm made for st that field names,
// made by mk and recorded the first time, unless another devvm recorded
// one meanwhile.
func made(ctx context.Context, st *state.VM, field func(*NetMade) *string, mk func() (string, error)) (string, error) {
	if n, err := Of(st); err != nil || *field(&n.Made) != "" {
		return *field(&n.Made), err
	}
	v, err := mk()
	if err != nil {
		return "", err
	}
	err = st.Update(ctx, func(st *state.VM) error {
		n, err := Of(st)
		if err != nil || *field(&n.Made) != "" {
			v = *field(&n.Made)
			return err
		}
		*field(&n.Made) = v
		return Set(st, n)
	})
	return v, err
}

// Policies returns the policies of the VM vm, of image img with these
// shared tools, in the order its proxy reads them. builtin is devvm's own,
// as paths in its files (assets.FS), which the proxy reads from the
// binary: base's, then each shared tool's in shared-tools/, then each
// built-in layer's in layers/. files is the user's: each of the image's
// own layers' policy file, next to its script, the user's own for every
// VM, the VM's own, and the hosts its secrets imply. A missing file has no
// rules.
func Policies(d paths.Dirs, vm string, img image.Name, tools []string) (builtin, files []string) {
	builtin = []string{"layers/base.policy"}
	for _, t := range tools {
		// Only a shared tool's name names a file of devvm's.
		if image.IsTool(t) {
			builtin = append(builtin, "shared-tools/"+t+".policy")
		}
	}
	for _, p := range img.Layers() {
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
// proxy reaches it there (proxy.Config's Links), so links change no VM's
// environment.
const NoProxy = "localhost,127.0.0.1,::1"
