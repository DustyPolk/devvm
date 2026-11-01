package proxy

import (
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A rule from a policy file. One per line, '#' starts a comment:
//
//	example.com       exactly that host
//	*.example.com     any subdomain, at any depth (not example.com itself)
//	**                any host
//	10.0.0.5, 10.0.0.0/8, fd00::/8   addresses (also needed to reach private ranges, see floor)
//	…:22              only that port; without a port, 443 and 80
//	!rule             deny; a deny beats any allow
//
// Matching is case-insensitive.
type rule struct {
	text   string
	deny   bool
	host   string       // "", "**", "*.example.com" or "example.com"
	prefix netip.Prefix // for address rules
	port   int          // 0: 443 and 80
}

func parseRule(line string) (rule, bool) {
	line = strings.ToLower(strings.TrimSpace(strings.SplitN(line, "#", 2)[0]))
	if line == "" {
		return rule{}, false
	}
	r := rule{text: line}
	if strings.HasPrefix(line, "!") {
		r.deny = true
		line = strings.TrimSpace(line[1:])
	}
	// A port suffix: host:port, 1.2.3.4:port, [v6]:port, or cidr:port.
	if h, p, err := net.SplitHostPort(line); err == nil {
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			line, r.port = h, n
		}
	} else if i := strings.LastIndex(line, ":"); i > 0 && strings.Contains(line, "/") && !strings.Contains(line[:i], ":") {
		if n, err := strconv.Atoi(line[i+1:]); err == nil && n > 0 && n < 65536 {
			line, r.port = line[:i], n
		}
	}
	if p, err := netip.ParsePrefix(line); err == nil {
		r.prefix = p.Masked()
		return r, true
	}
	if a, err := netip.ParseAddr(line); err == nil {
		r.prefix = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
		return r, true
	}
	if line == "" || strings.ContainsAny(line, " /\\") {
		return rule{}, false
	}
	r.host = line
	return r, true
}

// ValidRule reports whether s is one rule, as devvm network allow writes it: a
// host name, *.name, ** or an address or CIDR (no zone), optionally with a
// port and a leading !, and no spaces or comment. AllowRule's rules are
// valid.
func ValidRule(s string) bool {
	if s == "" || strings.ContainsAny(s, "# \t\r\n%") {
		return false
	}
	r, ok := parseRule(s)
	switch {
	case !ok:
		return false
	case r.prefix.IsValid():
		return true
	case r.host == "**":
		return true
	}
	h := strings.TrimPrefix(r.host, "*.")
	return !strings.ContainsAny(h, "[]") && hostOK(h)
}

func (r rule) portOK(port int) bool {
	if r.port == 0 {
		return port == 443 || port == 80
	}
	return r.port == port
}

// matchHost: does a host-name rule cover host (lower-case, no port)?
func (r rule) matchHost(host string) bool {
	switch {
	case r.host == "":
		return false
	case r.host == "**":
		return true
	case strings.HasPrefix(r.host, "*."):
		return strings.HasSuffix(host, r.host[1:])
	default:
		return r.host == host
	}
}

// matchAddr: does an address rule cover a? A NAT64 address also matches as the IPv4 address in it,
// as internal() judges it, so a deny for an IPv4 address holds on IPv6-only networks too.
func (r rule) matchAddr(a netip.Addr) bool {
	if !r.prefix.IsValid() {
		return false
	}
	a = a.Unmap()
	if r.prefix.Contains(a) {
		return true
	}
	v4, ok := nat64IPv4(a)
	return ok && r.prefix.Contains(v4)
}

// policy is the union of several files, each re-read when its mtime changes.
type policy struct {
	mu    sync.Mutex
	files []string
	mtime map[string]time.Time
	rules map[string][]rule
}

func newPolicy(files []string) *policy {
	return &policy{files: files, mtime: map[string]time.Time{}, rules: map[string][]rule{}}
}

func (p *policy) all() []rule {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []rule
	for _, f := range p.files {
		st, err := os.Stat(f)
		if err != nil {
			delete(p.rules, f) // a missing file has no rules
			continue
		}
		if !st.ModTime().Equal(p.mtime[f]) {
			b, err := os.ReadFile(f) //nolint:gosec // devvm names the policy files
			if err != nil {          // keep its last rules (a deny dropped for a moment would let through)
				out = append(out, p.rules[f]...)
				continue
			}
			var rs []rule
			for _, l := range strings.Split(string(b), "\n") {
				if r, ok := parseRule(l); ok {
					rs = append(rs, r)
				}
			}
			p.rules[f], p.mtime[f] = rs, st.ModTime()
			logf("policy %s: %d rules", f, len(rs))
		}
		out = append(out, p.rules[f]...)
	}
	return out
}

// decide whether host:port may be reached. host is a name or an address literal. It returns the
// matching rule's text, or why not.
func (p *policy) decide(host string, port int) (bool, string) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	rs := p.all()
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	isAddr := err == nil
	match := func(r rule) bool {
		if !r.portOK(port) {
			return false
		}
		if isAddr {
			return r.matchAddr(addr)
		}
		return r.matchHost(host)
	}
	for _, r := range rs {
		if r.deny && match(r) {
			return false, "denied by " + r.text
		}
	}
	for _, r := range rs {
		if !r.deny && match(r) {
			return true, r.text
		}
	}
	return false, "not on the allowlist"
}

// byName: an allow rule that names host, as itself or under *.domain, for port: one that's more
// than **, which allows any host.
func (p *policy) byName(host string, port int) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, r := range p.all() {
		if !r.deny && r.host != "**" && r.portOK(port) && r.matchHost(host) {
			return true
		}
	}
	return false
}

// addrAllowed: an explicit address rule for a (needed to pass the floor), and no deny for it.
func (p *policy) addrAllowed(a netip.Addr, port int) bool {
	if p.addrDenied(a, port) {
		return false
	}
	for _, r := range p.all() {
		if !r.deny && r.portOK(port) && r.matchAddr(a) {
			return true
		}
	}
	return false
}

// addrDenied: a deny rule for address a. A deny beats any allow, including a host rule whose name
// resolves to a, so every address the proxy dials is checked against it.
func (p *policy) addrDenied(a netip.Addr, port int) bool {
	for _, r := range p.all() {
		if r.deny && r.portOK(port) && r.matchAddr(a) {
			return true
		}
	}
	return false
}
