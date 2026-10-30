package lima

import (
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// portForward is a rule in Lima's portForwards.
type portForward struct {
	GuestIP        string `json:"guestIP,omitempty"`
	GuestPortRange [2]int `json:"guestPortRange"`
	Proto          string `json:"proto"`
	Ignore         bool   `json:"ignore,omitempty"`
}

// noForwardsSet is the --set that makes Lima forward no port of a VM's,
// TCP or UDP, on any address: devvm's forwarder forwards them (ports.go).
// Lima checks rules in order, and its own last rule forwards everything:
// an ignore rule for each of 127.0.0.1 (::1 too), 0.0.0.0 and :: comes
// first. Lima's ssh is its VM driver's forward, not a rule.
func noForwardsSet() string { return ".portForwards = " + yqJSON(noForwards()) }

// noForwards are noForwardsSet's rules.
func noForwards() []portForward {
	rules := []portForward{}
	for _, ip := range []string{"127.0.0.1", "0.0.0.0", "::"} {
		rules = append(rules, portForward{GuestIP: ip, GuestPortRange: [2]int{1, 65535}, Proto: "any", Ignore: true})
	}
	return rules
}

// forwardsNone reports whether a VM's rules, as limactl list has them,
// are noForwards(): limactl fills in the host side, so only what devvm
// set is compared.
func forwardsNone(have []portForward) bool {
	want := noForwards()
	if len(have) != len(want) {
		return false
	}
	for i, w := range want {
		h := have[i]
		if h.GuestPortRange != w.GuestPortRange || h.Proto != w.Proto || h.Ignore != w.Ignore || h.GuestIP != w.GuestIP {
			return false
		}
	}
	return true
}

// mountsSet is the --set for a VM's mounts. Paths with a comma are
// refused: Lima and QEMU split mount options on commas.
func mountsSet(ms []mount) (string, error) {
	for _, m := range ms {
		if strings.Contains(m.Location, ",") {
			return "", msg.MountComma(m.Location)
		}
	}
	if ms == nil {
		ms = []mount{}
	}
	return ".mounts = " + yqJSON(ms), nil
}

// readOnly returns read-only mounts of dirs, each at its host path.
func readOnly(dirs []string) []mount {
	var ms []mount
	for _, d := range dirs {
		ms = append(ms, mount{Location: d, Writable: ptr(false)})
	}
	return ms
}

func ptr[T any](v T) *T { return &v }
