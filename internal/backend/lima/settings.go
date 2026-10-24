package lima

import (
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// portForward is a rule in Lima's portForwards.
type portForward struct {
	GuestIP        string `json:"guestIP,omitempty"`
	GuestPortRange [2]int `json:"guestPortRange"`
	Proto          string `json:"proto"`
	Ignore         bool   `json:"ignore,omitempty"`
}

// portsSet is the --set for which ports a VM forwards: the listed ones,
// each to the same port on the host, then nothing else. Lima checks rules
// in order, and its own last rule forwards everything, so all is no rules
// at all. A rule without guestIP matches every address a server binds to,
// but an ignore rule needs one for each of 127.0.0.1, 0.0.0.0 and ::.
func portsSet(p backend.Ports) string {
	rules := []portForward{}
	if !p.All {
		for _, r := range p.Ranges {
			rules = append(rules, portForward{GuestPortRange: [2]int{r.From, r.To}, Proto: "any"})
		}
		for _, ip := range []string{"127.0.0.1", "0.0.0.0", "::"} {
			rules = append(rules, portForward{GuestIP: ip, GuestPortRange: [2]int{1, 65535}, Proto: "any", Ignore: true})
		}
	}
	return ".portForwards = " + yqJSON(rules)
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
