// Package proxy is the network policy point for one devvm VM, run as devvm _proxy.
//
// devvm starts one per VM, listening on a loopback port. The VM's Lima hostagent runs under a
// Seatbelt profile, or a restricted QEMU network, that lets the VM reach nothing else, so every
// connection the VM makes goes through here. Other programs on the host can reach the port too, so
// requests must carry the VM's password (-auth-file), and the VM's placeholders end in its own key:
//
//   - CONNECT host:port and absolute-form requests (GET http://host/…) are checked against the
//     policy files (see policy.go); allowed ones are tunneled or forwarded, untouched.
//   - Names are resolved here, and connections to loopback, private, link-local and the host's own
//     addresses are refused unless a rule names the address (the "floor").
//   - A tunnel that starts with a TLS ClientHello must name the CONNECT host in its SNI.
//   - Hosts with a bound secret (-secrets) are intercepted on :443 instead, to put the secret in
//     (see secrets.go). Everything else passes unread.
//   - Every decision goes to a JSON-lines log, and http://devvm.proxy/blocked lists recent blocks
//     (/blocked/new: those since the VM's last prompt; /ca.pem serves the CA).
//
// On a backend that asks the host about each connection a VM opens (smolvm), the proxy is also
// that decider (-decider, decider.go): it takes over connections to itself and refuses the rest.
// On one whose VMs can't reach the host's loopback (Docker), it also listens on a unix socket
// (-listen-unix), which a relay on the VM's own network reaches (-relay, relay.go).
//
// It exits when the VM's hostagent does (-lifeline). Standard library only: it prints nothing
// itself, but writes to the streams devvm hands Main.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	listen    = "127.0.0.1:0"
	vmName    = "vm"
	policies  = ""
	logPath   = ""
	lifeline  = ""
	caFile    = ""
	caKeyF    = ""
	initCADir = ""
	loginKind = ""
	loginFile = ""
	loginVM   = false
	loginBnd  = false
	loginGet  = ""
	redactFmt = ""
)

// newFlags defines devvm _proxy's flags on a flag set of its own, writing into the variables above.
// Each takes one dash or two.
func newFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("devvm _proxy", flag.ContinueOnError)
	fs.StringVar(&listen, "listen", "127.0.0.1:0", "address to listen on")
	fs.StringVar(&listenUnix, "listen-unix", "", "also listen on this unix socket, only the user's (relay.go)")
	fs.StringVar(&relayAddr, "relay", "", "relay mode: listen on this address and pass each connection, unread, to -relay-to (relay.go)")
	fs.StringVar(&relayTo, "relay-to", "", "with -relay: the proxy's unix socket")
	fs.StringVar(&vmName, "vm", "vm", "VM name, for logs")
	fs.StringVar(&policies, "policy", "", "policy files, comma-separated (missing files have no rules)")
	fs.StringVar(&logPath, "log", "", "JSON-lines event log (default: stdout)")
	fs.StringVar(&lifeline, "lifeline", "", "pid file of the VM's hostagent: exit when that process is gone")
	fs.StringVar(&caFile, "ca", "", "CA certificate for interception (PEM); with -cakey")
	fs.StringVar(&caKeyF, "cakey", "", "CA private key (PEM)")
	fs.StringVar(&upstreamCAFile, "upstream-cas", "", "CA certificates (PEM) trusted for the proxy's own TLS connections, besides the system's (secrets.go)")
	fs.StringVar(&initCADir, "init-ca", "", "write a new CA (ca.pem, ca.key) into this directory unless one is there, and exit")
	fs.StringVar(&loginKind, "login", "", "codex, opencode or claude: work on that agent's login file (logins.go), with -login-file and one of the next three")
	fs.StringVar(&loginFile, "login-file", "", "the agent's login file on the host")
	fs.BoolVar(&loginVM, "login-vm", false, "print the VM's copy of the login file, with placeholders")
	fs.BoolVar(&loginBnd, "login-bindings", false, "print the login's secret bindings (JSON, no values)")
	fs.StringVar(&loginGet, "login-get", "", "print one value (refreshing an OAuth token when due)")
	fs.StringVar(&redactFmt, "redact", "", "json or toml: copy stdin to stdout with secret-named values replaced, their paths on stderr (redact.go)")
	fs.StringVar(&secretsPath, "secrets", "", "secret bindings (JSON, no values), re-read on change")
	fs.StringVar(&secretCmd, "secret-cmd", "", "the devvm binary, run as `devvm secret get NAME`")
	fs.StringVar(&loginSalt, "login-salt", "", "with -login-vm or -login-bindings: the VM's key, which ends its placeholders (default $DEVVM_LOGIN_SALT, which keeps it out of the process's arguments)")
	fs.BoolVar(&loginSecrets, "login-secrets", false, "with -login claude: print the MCP servers whose settings hold secrets")
	fs.BoolVar(&loginHosts, "login-hosts", false, "with -login-bindings: print the bindings' hosts as allowlist lines (host:443) instead")
	fs.StringVar(&authFile, "auth-file", "", "file holding the VM's password: requests need Proxy-Authorization for user devvm with it")
	fs.StringVar(&deciderAddr, "decider", "", "also decide, on this loopback address, each connection the VM opens (decider.go)")
	fs.StringVar(&deciderTo, "decider-to", "", "with -decider: the proxy's address in the VM, the only destination taken over")
	fs.StringVar(&deciderTokenFile, "decider-token-file", "", "with -decider: file holding the token the backend proves itself with (64 hex digits)")
	return fs
}

// --- who may use the proxy ---

var (
	authFile  = ""
	proxyAuth = "" // the VM's password; empty: no check
)

// authorized: the request carries the VM's password (Basic, user devvm). Lima's hostagent makes the
// VM's connections, so the port alone can't tell the VM apart from anything else on the host that
// reaches it: other VMs, other users. Inside an intercepted tunnel there's no proxy header: the
// CONNECT that opened it was checked.
func authorized(r *http.Request) bool {
	if proxyAuth == "" {
		return true
	}
	v := r.Header.Get("Proxy-Authorization")
	if len(v) < 6 || !strings.EqualFold(v[:6], "basic ") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v[6:]))
	return err == nil && subtle.ConstantTimeCompare(raw, []byte("devvm:"+proxyAuth)) == 1
}

var pol *policy

// --- events ---

// Event is one line of the event log, as JSON: a decision about a
// connection or a request. devvm log reads them.
type Event struct {
	Time    string   `json:"time"`
	VM      string   `json:"vm"`
	Event   string   `json:"event"` // tunnel, intercept, forward, blocked
	Host    string   `json:"host"`
	Port    int      `json:"port"`
	Method  string   `json:"method,omitempty"`
	Path    string   `json:"path,omitempty"`
	Rule    string   `json:"rule,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Agent   string   `json:"agent,omitempty"`
	Secrets []string `json:"secrets,omitempty"` // names of secrets put in, never values
	Refused bool     `json:"refused,omitempty"` // blocked whatever the allowlist says: no rule allows it
}

// A block and its number (the first is 1), for /blocked/new.
type numbered struct {
	n int
	e Event
}

var (
	logMu  sync.Mutex
	logOut io.Writer  = io.Discard // Main sets it: -log, else stdout
	recent []numbered              // the last blocks, for /blocked
	blocks int                     // blocks so far
)

func record(e Event) {
	e.Time, e.VM = time.Now().UTC().Format(time.RFC3339), vmName
	b, _ := json.Marshal(e)
	logMu.Lock()
	defer logMu.Unlock()
	fmt.Fprintln(logOut, string(b))
	if e.Event == "blocked" {
		blocks++
		recent = append(recent, numbered{blocks, e})
		if len(recent) > 50 {
			recent = recent[len(recent)-50:]
		}
	}
}

// newBlocks answers /blocked/new?after=N for the VM's prompt: the number of blocks so far on the first
// line, then the blocks after the Nth, each rule once, and the command that allows them. No N (a new
// shell) means none; an N past the count means the proxy restarted, so every block since counts.
func newBlocks(rs []numbered, count int, after string) string {
	var b strings.Builder
	fmt.Fprintln(&b, count)
	n, err := strconv.Atoi(after)
	if err != nil {
		return b.String()
	}
	if n > count {
		n = 0
	}
	var rules []string
	seen := map[string]bool{}
	for _, r := range rs {
		if r.n <= n {
			continue
		}
		if r.e.Refused {
			fmt.Fprintf(&b, "devvm: refused %s %s%s (%s)\n", r.e.Method, r.e.Host, r.e.Path, r.e.Reason)
			continue
		}
		rule := ruleFor(r.e.Host, r.e.Port)
		if seen[rule] {
			continue
		}
		seen[rule] = true
		rules = append(rules, rule)
		fmt.Fprintf(&b, "devvm: blocked %s:%d (%s)\n", r.e.Host, r.e.Port, r.e.Reason)
	}
	if len(rules) > 0 {
		fmt.Fprintf(&b, "To allow, run this on the host:\ndevvm policy allow %s\n", strings.Join(rules, " "))
	}
	return b.String()
}

// logger: the proxy's own log lines (what it loaded, what failed), not events. Main points it at
// the stderr devvm hands it.
var logger = log.New(io.Discard, "devvm proxy: ", log.LstdFlags)

func logf(format string, a ...any) { logger.Printf(format, a...) }

// --- destinations ---

// The host's own addresses, re-read every 30 seconds: they change (a new network, IPv6 temporary
// addresses).
var (
	ownMu    sync.Mutex
	ownAddrs = map[netip.Addr]bool{}
	ownAt    time.Time
)

func loadOwnAddrs() {
	addrs, _ := net.InterfaceAddrs()
	own := map[netip.Addr]bool{}
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			own[p.Addr().Unmap()] = true
		}
	}
	ownMu.Lock()
	ownAddrs, ownAt = own, time.Now()
	ownMu.Unlock()
}

func isOwn(a netip.Addr) bool {
	ownMu.Lock()
	stale := time.Since(ownAt) > 30*time.Second
	ownMu.Unlock()
	if stale {
		loadOwnAddrs()
	}
	ownMu.Lock()
	defer ownMu.Unlock()
	return ownAddrs[a]
}

// Ranges that aren't the public internet beyond what netip's predicates cover: CGNAT, "this network",
// IETF protocol assignments, benchmarking, reserved, local-use NAT64, IPv4-compatible IPv6 (::a.b.c.d,
// deprecated) and site-local IPv6 (deprecated, still routed by some systems).
var notPublic = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("fec0::/10"),
}
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// nat64IPv4: the IPv4 address in a NAT64 address (64:ff9b::a.b.c.d).
func nat64IPv4(a netip.Addr) (netip.Addr, bool) {
	if !nat64.Contains(a) {
		return netip.Addr{}, false
	}
	b := a.As16()
	return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
}

// internal: addresses a VM must not reach without an explicit rule: the host itself and its networks.
// A NAT64 address (64:ff9b::a.b.c.d, on IPv6-only networks) is judged by the IPv4 address in it.
func internal(a netip.Addr) bool {
	a = a.Unmap()
	if v4, ok := nat64IPv4(a); ok {
		return internal(v4)
	}
	for _, p := range notPublic {
		if p.Contains(a) {
			return true
		}
	}
	return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast() || isOwn(a)
}

var errFloor = errors.New("resolves to an internal address")

// resolve host to the addresses that may be dialed for port. Callers dial these, not the name, so
// a second DNS answer can't swap in an internal address.
func resolve(ctx context.Context, host string, port int) ([]netip.Addr, error) {
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		addrs = []netip.Addr{a.Unmap()}
	} else {
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, a := range ips {
			addrs = append(addrs, a.Unmap())
		}
	}
	var ok []netip.Addr
	for _, a := range addrs {
		if !pol.addrDenied(a, port) && (!internal(a) || pol.addrAllowed(a, port)) {
			ok = append(ok, a)
		}
	}
	if len(ok) == 0 {
		return nil, errFloor
	}
	return ok, nil
}

func dial(ctx context.Context, host string, port int) (net.Conn, error) {
	addrs, err := resolve(ctx, host, port)
	if err != nil {
		return nil, err
	}
	if port < 1 || port > 65535 { // the policy allows none of these; never dial one wrapped around
		return nil, fmt.Errorf("port %d", port)
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	for _, a := range addrs {
		c, e := d.DialContext(ctx, "tcp", netip.AddrPortFrom(a, uint16(port)).String())
		if e == nil {
			return c, nil
		}
		err = e
	}
	return nil, err
}

// --- the proxy ---

// hostOK: a host name (letters, digits, '-', '_', '.') or an address. The VM chooses the host, and it
// ends up in the log, in the policy and in the allow command devvm suggests for the host's shell.
func hostOK(h string) bool {
	if a, err := netip.ParseAddr(strings.Trim(h, "[]")); err == nil {
		return a.Zone() == "" // a zone (fe80::1%eth0) is any text
	}
	if h == "" || len(h) > 253 {
		return false
	}
	for _, c := range h {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

// splitHostPort: the host, lowercased and without a trailing dot (so the policy, bindings, SNI check
// and log all see one form), and the port (def when there's none).
func splitHostPort(hp string, def int) (string, int) {
	norm := func(h string) string { return strings.TrimSuffix(strings.ToLower(h), ".") }
	h, p, err := net.SplitHostPort(hp)
	if err != nil {
		return norm(strings.Trim(hp, "[]")), def
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return norm(h), def
	}
	return norm(h), n
}

func block(w http.ResponseWriter, e Event) {
	e.Event = "blocked"
	record(e)
	w.Header().Set("X-Devvm-Blocked", e.Host)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	fmt.Fprintf(w, "devvm: %s:%d is blocked (%s). Allow it on the host with: devvm policy allow %s\n",
		e.Host, e.Port, e.Reason, ruleFor(e.Host, e.Port))
}

func ruleFor(host string, port int) string {
	if port == 443 || port == 80 {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// AllowRule is the rule that allows host:port, for a devvm policy allow
// command; false for a host the policy doesn't take, which a logged event
// can't have, or a log that was edited.
func AllowRule(host string, port int) (string, bool) {
	if !hostOK(host) || port < 1 || port > 65535 {
		return "", false
	}
	return ruleFor(host, port), true
}

// check runs the policy and the floor; on success it returns the rule that allowed host:port.
func check(ctx context.Context, host string, port int) (string, string) {
	ok, why := pol.decide(host, port)
	if !ok {
		return "", why
	}
	if _, err := resolve(ctx, host, port); err != nil {
		if errors.Is(err, errFloor) {
			return "", "it " + err.Error() + "; allow the address itself to reach it"
		}
		return "", "can't resolve it: " + err.Error()
	}
	return why, ""
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	host, port := splitHostPort(r.Host, 443)
	if !hostOK(host) {
		http.Error(w, "devvm proxy: not a host name", http.StatusBadRequest)
		return
	}
	e := Event{Host: host, Port: port, Method: "CONNECT"}
	rule, why := check(r.Context(), host, port)
	if why != "" {
		e.Reason = why
		block(w, e)
		return
	}
	e.Rule = rule
	if bs := bindingsFor(host); interceptPorts[port] && len(bs) > 0 && caCert != nil {
		client, buf, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		_, _ = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		intercept(r.Context(), bufConn{client, buf.Reader}, host, port, e)
		return
	}
	up, err := dial(r.Context(), host, port)
	if err != nil {
		e.Reason = "connect failed: " + err.Error()
		block(w, e)
		return
	}
	defer up.Close()
	client, buf, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	_, _ = client.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

	// TLS: the SNI must be the host that was checked (blunts domain fronting). Other protocols
	// (SSH on :22, …) pass as they are.
	first, sni, isTLS := peekClientHello(buf.Reader)
	if isTLS && !sniMatches(sni, host) {
		e.Event, e.Reason = "blocked", fmt.Sprintf("TLS name %q doesn't match %s", sni, host)
		record(e)
		return
	}
	e.Event = "tunnel"
	record(e)
	_, _ = up.Write(first)
	go func() {
		_, _ = io.Copy(up, buf.Reader) // also drains anything bufio already read
		if t, ok := up.(*net.TCPConn); ok {
			_ = t.CloseWrite()
		}
	}()
	_, _ = io.Copy(client, up)
}

// bufConn: a hijacked connection whose first bytes may already sit in the server's bufio.Reader.
type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (c bufConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func sniMatches(sni, host string) bool {
	if _, err := netip.ParseAddr(host); err == nil {
		return sni == "" || strings.EqualFold(sni, host)
	}
	return strings.EqualFold(strings.TrimSuffix(sni, "."), host)
}

// peekClientHello reads the first TLS record if the stream starts with one, and returns the bytes
// read (to replay upstream), the SNI, and whether it was TLS.
func peekClientHello(r *bufio.Reader) ([]byte, string, bool) {
	hdr, err := r.Peek(5)
	if err != nil || hdr[0] != 0x16 || hdr[1] != 3 {
		return nil, "", false
	}
	n := int(hdr[3])<<8 | int(hdr[4])
	rec := make([]byte, 5+n)
	if _, err := io.ReadFull(r, rec); err != nil {
		return rec, "", true
	}
	return rec, sniOf(rec), true
}

// handleHTTP serves absolute-form requests (plain HTTP, or "GET https://…" from clients like
// busybox wget) and the reserved http://devvm.proxy/ endpoints.
func handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Host == "" {
		http.Error(w, "devvm proxy: not a proxy request", http.StatusBadRequest)
		return
	}
	scheme := "http"
	if r.URL.Scheme == "https" {
		scheme = "https"
	}
	def := 80
	if scheme == "https" {
		def = 443
	}
	host, port := splitHostPort(r.URL.Host, def)
	if !hostOK(host) {
		http.Error(w, "devvm proxy: not a host name", http.StatusBadRequest)
		return
	}
	if host == "devvm.proxy" {
		serveReserved(w, r)
		return
	}
	e := Event{Host: host, Port: port, Method: r.Method, Path: r.URL.Path, Agent: r.UserAgent()}
	if _, why := check(r.Context(), host, port); why != "" {
		e.Reason = why
		block(w, e)
		return
	}
	// Some clients half-close after sending; don't treat that as the client going away.
	forwarder(scheme, r.URL.Host, host, port).ServeHTTP(noCloseNotify{w}, r.WithContext(context.WithoutCancel(r.Context())))
}

type noCloseNotify struct{ http.ResponseWriter }

func (w noCloseNotify) Flush() { _ = http.NewResponseController(w.ResponseWriter).Flush() }

func serveReserved(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch r.URL.Path {
	case "/blocked":
		logMu.Lock()
		rs := append([]numbered(nil), recent...)
		logMu.Unlock()
		if len(rs) == 0 {
			fmt.Fprintln(w, "devvm: nothing blocked recently")
			return
		}
		var b bytes.Buffer
		for _, r := range rs {
			e := r.e
			if e.Refused {
				fmt.Fprintf(&b, "%s  %s %s%s  refused: %s\n", e.Time, e.Method, e.Host, e.Path, e.Reason)
				continue
			}
			fmt.Fprintf(&b, "%s  %s:%d  %s\n    allow on the host: devvm policy allow %s\n", e.Time, e.Host, e.Port, e.Reason, ruleFor(e.Host, e.Port))
		}
		_, _ = w.Write(b.Bytes())
	case "/blocked/new":
		logMu.Lock()
		s := newBlocks(recent, blocks, r.URL.Query().Get("after"))
		logMu.Unlock()
		fmt.Fprint(w, s)
	case "/ca.pem":
		if p := caPEM(); p != nil {
			_, _ = w.Write(p)
			return
		}
		http.Error(w, "devvm proxy: no CA", http.StatusNotFound)
	default:
		http.Error(w, "devvm proxy: unknown endpoint", http.StatusNotFound)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	if !authorized(r) {
		w.Header().Set("Proxy-Authenticate", `Basic realm="devvm"`)
		http.Error(w, "devvm proxy: this proxy belongs to another VM", http.StatusProxyAuthRequired)
		return
	}
	if r.Method == http.MethodConnect {
		handleConnect(w, r)
		return
	}
	handleHTTP(w, r)
}

// lifelineTick: how often watchLifeline looks (tests shorten it).
var lifelineTick = 3 * time.Second

// watchLifeline returns once the hostagent named by the pid file is gone, or never appears. It
// follows the first hostagent it sees, not later ones: a restarted VM gets a new proxy. A pid file
// written since it started whose process is already gone (the VM stopped between two looks) is a
// hostagent gone too; an older one is an earlier start's, and waited past.
func watchLifeline(path string) {
	start, pid := time.Now(), 0
	for range time.Tick(lifelineTick) {
		if pid == 0 {
			fi, serr := os.Stat(path)
			if b, err := os.ReadFile(path); err == nil && serr == nil { //nolint:gosec // devvm names the pid file
				// The pid is the first line: smolvm's has its start time on a second.
				first, _, _ := strings.Cut(string(b), "\n")
				if p, err := strconv.Atoi(strings.TrimSpace(first)); err == nil && p > 0 {
					if syscall.Kill(p, 0) == nil {
						pid = p
					} else if !fi.ModTime().Before(start) {
						return
					}
				}
			}
		}
		if (pid != 0 && syscall.Kill(pid, 0) != nil) || (pid == 0 && time.Since(start) > 15*time.Minute) {
			return
		}
	}
}

// IO is what devvm _proxy reads and writes: devvm's own streams.
type IO struct {
	Stdin io.Reader
	// Stdout gets data: the event log without -log, and what -login, -redact and -help print.
	Stdout io.Writer
	// Stderr gets the proxy's own log lines, and -redact's paths.
	Stderr io.Writer
	// Kept is told, with -login-vm, what of an agent's login file the VM's copy holds as it is:
	// Codex's auth.json keys or opencode's providers devvm doesn't know. Nil: nothing is told.
	Kept func(agent string, names []string)
}

// Main runs devvm _proxy with args (its flags): the proxy itself, until the VM's hostagent is
// gone, or one of the modes that do one thing and return (-init-ca, -login, -redact). -help prints
// the flags to std.Stdout. Errors are returned, not printed.
func Main(args []string, std IO) error {
	fs := newFlags()
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.SetOutput(std.Stdout)
			fs.PrintDefaults()
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if loginSalt == "" {
		loginSalt = os.Getenv("DEVVM_LOGIN_SALT")
	}
	logger.SetOutput(std.Stderr)
	warnKept = std.Kept
	if loginKind != "" {
		out, err := loginCmd(loginKind, loginFile, loginVM, loginBnd, loginGet)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(std.Stdout, out)
		return err
	}
	if redactFmt != "" {
		return redactCmd(redactFmt, std.Stdin, std.Stdout, std.Stderr)
	}
	if initCADir != "" {
		return initCA(initCADir)
	}
	if relayAddr != "" {
		return runRelay(relayAddr, relayTo)
	}
	if caFile != "" {
		if err := loadCA(caFile, caKeyF); err != nil {
			return err
		}
	}
	if upstreamCAFile != "" {
		if err := loadUpstreamCAs(upstreamCAFile); err != nil {
			return err
		}
	}
	// Without a password any program on the host could use the proxy, and the VM's tokens with it.
	if authFile == "" {
		return errors.New("-auth-file is required")
	}
	{
		b, err := os.ReadFile(authFile)
		if err != nil {
			return err
		}
		if proxyAuth = strings.TrimSpace(string(b)); proxyAuth == "" {
			return fmt.Errorf("%s is empty", authFile)
		}
	}
	logOut = std.Stdout
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		logOut = f
	}
	var files []string
	for f := range strings.SplitSeq(policies, ",") {
		if f = strings.TrimSpace(f); f != "" {
			files = append(files, f)
		}
	}
	pol = newPolicy(files)
	pol.all()
	loadOwnAddrs()
	// The decider listens first: "listening on" means both are.
	h, to, token, err := startDecider()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", listen) //nolint:noctx // listens for the proxy's whole life
	if err != nil {
		if h != nil {
			_ = h.Close()
		}
		return err
	}
	srv := &http.Server{Handler: http.HandlerFunc(handler), ReadHeaderTimeout: 30 * time.Second}
	if listenUnix != "" {
		uln, err := listenOn(listenUnix)
		if err != nil {
			_ = ln.Close()
			if h != nil {
				_ = h.Close()
			}
			return err
		}
		defer func() { _ = uln.Close() }()
		go func() { _ = srv.Serve(uln) }()
	}
	if h != nil {
		defer func() { _ = h.Close() }()
		logf("deciding on %s for %s, taking over %s", h.Addr(), vmName, to)
		go serveDecider(h, token, to)
		go func() { _ = srv.Serve(h) }()
	}
	logf("listening on %s for %s", ln.Addr(), vmName)
	if lifeline != "" {
		go func() {
			watchLifeline(lifeline)
			logf("the VM's hostagent is gone; exiting")
			_ = srv.Close()
		}()
	}
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
