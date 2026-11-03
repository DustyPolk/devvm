package smolvm

import (
	"net/netip"
	"time"

	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
)

// newForwards is how smolvm's VMs' forwarders run: devvm _smolvm-ports,
// self, its files in each VM's backend folder.
func newForwards(d paths.Dirs, self string) vmssh.Forwards {
	return vmssh.Forwards{
		Dirs: d, Prefix: "smolvm", Cmd: forwarderCmd, Self: self,
		Detach: daemon.Detach, Args: platform.ProcessArgs, Tick: 500 * time.Millisecond,
	}
}

// deciderPort is the port of a decider's address (127.0.0.1:PORT), 0 for
// none.
func deciderPort(addr string) int {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return 0
	}
	return int(ap.Port())
}
