package netpolicy

import (
	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/proxy"
	"github.com/devvmhq/devvm/internal/settings"
	"github.com/devvmhq/devvm/internal/state"
)

// A VM's own transparent mode (Net.Transparent), as devvm network
// transparent sets it.
const (
	TransparentOn  = "on"
	TransparentOff = "off"
)

// TransparentChoices are the values devvm network transparent takes.
func TransparentChoices() []string { return []string{TransparentOn, TransparentOff} }

// Where a VM's transparent mode comes from (Transparent.From).
const (
	FromVM      = "vm"      // its own, devvm network transparent's
	FromConfig  = "config"  // config.toml's transparent-proxy
	FromDefault = "default" // neither: on
)

// Transparent is a VM's transparent mode: whether it's on when the VM runs
// confined, and where that comes from (FromVM, FromConfig, FromDefault).
type Transparent struct {
	On   bool
	From string
}

// TransparentOf is st's transparent mode: its own (Net.Transparent), else
// config.toml's transparent-proxy (settings.TransparentProxy), else on. A
// section that can't be read reads as the default, as its tier reads as
// none.
func TransparentOf(d paths.Dirs, st *state.VM) (Transparent, error) {
	if n, err := Of(st); err == nil && n.Transparent != "" {
		return Transparent{On: n.Transparent == TransparentOn, From: FromVM}, nil
	}
	on, set, err := settings.TransparentProxy(d)
	switch {
	case err != nil:
		return Transparent{}, err
	case set:
		return Transparent{On: on, From: FromConfig}, nil
	}
	return Transparent{On: true, From: FromDefault}, nil
}

// SetTransparent sets st's own transparent mode, TransparentOn or
// TransparentOff, in st alone: inside st.Update.
func SetTransparent(st *state.VM, v string) error {
	n, err := Of(st)
	if err != nil {
		return err
	}
	n.Transparent = v
	return Set(st, n)
}

// TransparentAddr is the stand-in address a VM's setup gives every name
// (proxy.SentinelAddr) when its transparent mode applies: it runs confined
// to its proxy (conf, the confinement it runs with, which a tier's change
// into or out of open changes only at its next start) and t is on. "" when
// it doesn't, and the setup turns transparent mode off.
func TransparentAddr(conf backend.Confinement, t Transparent) string {
	if conf.Net != backend.NetProxy || !t.On {
		return ""
	}
	return proxy.SentinelAddr
}
