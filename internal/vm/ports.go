package vm

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// Ports values: every port the VM listens on is forwarded, or none.
const (
	PortsAll  = "all"
	PortsNone = "none"
)

var portEntry = regexp.MustCompile(`^([0-9]{1,5})(-([0-9]{1,5}))?$`)

// ParsePorts checks which ports to forward, as --ports or the ports file
// give them: all, none, or ports and ranges (5173,8000-8010). It returns
// the value without spaces or repeats.
func ParsePorts(v string) (string, error) {
	s := strings.ReplaceAll(v, " ", "")
	if s == PortsAll || s == PortsNone {
		return s, nil
	}
	bad := msg.PortInvalid(strconv.Quote(v))
	if s == "" {
		return "", bad
	}
	var out []string
	for e := range strings.SplitSeq(s, ",") {
		m := portEntry.FindStringSubmatch(e)
		if m == nil {
			return "", bad
		}
		a, _ := strconv.Atoi(m[1])
		b := a
		if m[3] != "" {
			b, _ = strconv.Atoi(m[3])
		}
		if a < 1 || a > b || b > 65535 {
			return "", bad
		}
		e = strconv.Itoa(a)
		if a != b {
			e += "-" + strconv.Itoa(b)
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return strings.Join(out, ","), nil
}

// PortsBackend returns a parsed ports value as the backend has it: "" is
// all, Lima's default.
func PortsBackend(v string) backend.Ports {
	switch v {
	case "", PortsAll:
		return backend.Ports{All: true}
	case PortsNone:
		return backend.Ports{}
	}
	var p backend.Ports
	for e := range strings.SplitSeq(v, ",") {
		a, b, ok := strings.Cut(e, "-")
		from, _ := strconv.Atoi(a)
		to := from
		if ok {
			to, _ = strconv.Atoi(b)
		}
		p.Ranges = append(p.Ranges, backend.PortRange{From: from, To: to})
	}
	return p
}
