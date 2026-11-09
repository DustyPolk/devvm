package settings

import "github.com/devvmhq/devvm/internal/paths"

// KeyTransparentProxy is config.toml's switch for transparent mode: on a
// confined VM, the names of hosts that tools without proxy settings look up
// lead to the VM's proxy, which judges each connection by the name in it.
const KeyTransparentProxy = "transparent-proxy"

// TransparentProxy reports config.toml's transparent-proxy, and whether it
// sets it: on, unless it's false. A VM's own setting comes first
// (netpolicy.TransparentOf).
func TransparentProxy(d paths.Dirs) (on, set bool, err error) {
	cf, err := decodeConfig(d.ConfigFile())
	if err != nil {
		return false, false, err
	}
	return cf.TransparentProxy.v != "false", cf.TransparentProxy.set, nil
}
