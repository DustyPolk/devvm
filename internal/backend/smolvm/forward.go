package smolvm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/platform"
)

// Forwarding every port (ports-all) is emulated, and reviewed:
// smolvm forwards only the ports a machine was made or updated with, at
// most 64, and only to servers listening on all of the guest's addresses.
// So a VM that forwards every port gets none of smolvm's, and a forwarder
// of devvm's on the host instead (devvm _smolvm-ports, run detached by
// Start, one per VM):
//
//   - it reaches the VM only through the VM's sshd (ssh.go), as the VM's
//     user with its own key, the host key read through smolvm at the start
//     and pinned: one ssh connection (a master), whose session runs
//     ports.sh, which reports what listens in the VM; each forwarded
//     connection is a channel of it (ssh -W through the master), which the
//     VM's sshd opens to the VM's own loopback;
//   - it listens on the host's loopback alone (127.0.0.1 and ::1), on the
//     port the VM listens on, for servers on the VM's loopback or all its
//     addresses, from 1024 up; never on a port the host has on any address
//     (no SO_REUSEADDR: platform.ListenExclusive), nor on a port devvm
//     records for any VM (ssh, proxy, decider, listed forwards);
//   - every connection starts on the host: the VM opens none to the host
//     through it, and the master asks for no forwarding into the host;
//   - it ends when the machine's process does (its lifeline, agent.pid),
//     and devvm stops it when it stops the VM.

// Limits on what the VM's report makes the forwarder do.
const (
	minForwardPort = 1024 // privileged ports aren't forwarded
	maxForwards    = 256  // ports forwarded at once
	maxListeners   = 4096 // lines in one report
	maxRelays      = 256  // connections carried at once
)

// Forwarder forwards a running VM's ports to the host's loopback, as the
// VM reports them (Watch).
type Forwarder struct {
	VM string
	// Lifeline is the machine's agent.pid: the forwarder ends once what
	// it holds changes, or its process is gone.
	Lifeline string
	// Reserved returns the host ports never forwarded, asked each time a
	// port would be; while it fails, none is.
	Reserved func() (map[int]bool, error)
	// Watch connects to the VM and runs ports.sh there, returning its
	// output and how to wait for it; it ends with ctx or the connection.
	Watch func(ctx context.Context) (io.ReadCloser, func() error, error)
	// Relay carries conn to host:port in the VM until either side ends,
	// then closes conn.
	Relay func(ctx context.Context, conn net.Conn, host string, port int)
	// Listen listens on a host loopback address (HOST:PORT).
	Listen func(ctx context.Context, addr string) (net.Listener, error)
	// Log gets its lines.
	Log io.Writer
	// Tick is how often the lifeline is looked at, and how long a lost
	// connection waits to be made again.
	Tick time.Duration
	// Gap is the least time between two reports applied: one that comes
	// sooner waits, and the VM's ssh channel with it.
	Gap time.Duration

	mu      sync.Mutex
	ports   map[int]*forward
	skipped map[int]bool // not forwarded: tried again only once the VM's listener goes and comes back
	relays  chan struct{}
}

// forward is a port being forwarded: its host listeners, and the address
// in the VM its connections go to.
type forward struct {
	lns  []net.Listener
	host string
}

func (f *Forwarder) logf(format string, args ...any) {
	_, _ = fmt.Fprintf(f.Log, "devvm ports: "+format+"\n", args...)
}

// Run forwards the VM's ports until ctx ends or the lifeline does, making
// the connection again whenever it's lost.
func (f *Forwarder) Run(ctx context.Context) error {
	life, err := readLifeline(f.Lifeline)
	if err != nil || life == "" {
		return fmt.Errorf("%s: no lifeline at %s", f.VM, f.Lifeline)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer cancel()
		for lifelineHolds(f.Lifeline, life) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(f.Tick):
			}
		}
		f.logf("%s stopped", f.VM)
	}()
	f.ports, f.skipped, f.relays = map[int]*forward{}, map[int]bool{}, make(chan struct{}, maxRelays)
	defer f.closeAll()
	for ctx.Err() == nil {
		if err := f.session(ctx); err != nil && ctx.Err() == nil {
			f.logf("%s: %v", f.VM, err)
		}
		f.closeAll()
		select {
		case <-ctx.Done():
		case <-time.After(f.Tick):
		}
	}
	return nil
}

// session makes one connection to the VM and forwards what it reports
// until it ends.
func (f *Forwarder) session(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, wait, err := f.Watch(ctx)
	if err != nil {
		return err
	}
	defer func() { cancel(); _ = wait() }()
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 128), 128)
	var report []string
	var last time.Time
	ready := false
	for sc.Scan() {
		if line := sc.Text(); line != "." {
			if len(report) >= maxListeners {
				return errors.New("its report is too long")
			}
			report = append(report, line)
			continue
		}
		if err := waitUntil(ctx, last.Add(f.Gap)); err != nil {
			return err
		}
		last = time.Now()
		f.apply(ctx, listeners(report))
		report = report[:0]
		if !ready {
			f.logf("forwarding %s's ports", f.VM)
			ready = true
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errors.New("the connection ended")
}

// waitUntil waits until t, or ctx ends.
func waitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// What the VM can make the forwarder log (logLimit): logBurst writes at
// once, then one per logEvery, each at most logWriteMax bytes.
const (
	logBurst    = 200
	logEvery    = 10 * time.Second
	logWriteMax = 512
)

// logLimit is the forwarder's log, which the VM can make it write to (its
// ports, its ssh's errors, and its sshd's stderr): limited, so it can't
// fill the host's disk, and control characters but newlines and tabs
// shown as '?'.
type logLimit struct {
	w   io.Writer
	now func() time.Time

	mu      sync.Mutex
	tokens  int
	at      time.Time
	dropped int
}

func newLogLimit(w io.Writer, now func() time.Time) *logLimit {
	return &logLimit{w: w, now: now, tokens: logBurst, at: now()}
}

func (l *logLimit) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := int(l.now().Sub(l.at) / logEvery); n > 0 {
		l.tokens, l.at = min(logBurst, l.tokens+n), l.at.Add(time.Duration(n)*logEvery)
	}
	if l.tokens == 0 {
		l.dropped++
		return len(p), nil
	}
	l.tokens--
	var b []byte
	if l.dropped > 0 {
		b = fmt.Appendf(b, "devvm ports: %d writes not logged\n", l.dropped)
		l.dropped = 0
	}
	for _, c := range p[:min(len(p), logWriteMax)] {
		if c < 0x20 && c != '\n' && c != '\t' || c == 0x7f {
			c = '?'
		}
		b = append(b, c)
	}
	if len(p) > logWriteMax {
		b = append(b, "…\n"...)
	}
	_, err := l.w.Write(b)
	return len(p), err
}

// apply makes the forwards what the VM reports: port to address in the
// VM.
func (f *Forwarder) apply(ctx context.Context, want map[int]string) {
	reserved, rerr := f.Reserved()
	if rerr != nil {
		f.logf("no new ports forwarded: devvm's ports unknown: %v", rerr)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for port, fw := range f.ports {
		if _, ok := want[port]; !ok {
			fw.close()
			delete(f.ports, port)
			f.logf("port %d: no longer forwarded", port)
		}
	}
	for port := range f.skipped {
		if _, ok := want[port]; !ok {
			delete(f.skipped, port)
		}
	}
	for _, port := range slices.Sorted(maps.Keys(want)) {
		host := want[port]
		if fw, ok := f.ports[port]; ok {
			fw.host = host
			continue
		}
		if rerr != nil || port < minForwardPort || reserved[port] || f.skipped[port] {
			continue
		}
		f.skipped[port] = true
		if len(f.ports) >= maxForwards {
			f.logf("port %d: not forwarded: %d forwarded already", port, maxForwards)
			continue
		}
		fw, err := f.open(ctx, port)
		if err != nil {
			f.logf("port %d: not forwarded: %v", port, err)
			continue
		}
		fw.host = host
		f.ports[port] = fw
		delete(f.skipped, port)
		f.logf("port %d: forwarded", port)
	}
}

// open listens on the port on the host's loopback, IPv4 and IPv6 (a host
// without IPv6 gets IPv4 alone), and serves it.
func (f *Forwarder) open(ctx context.Context, port int) (*forward, error) {
	p := strconv.Itoa(port)
	l4, err := f.Listen(ctx, net.JoinHostPort("127.0.0.1", p))
	if err != nil {
		return nil, err
	}
	fw := &forward{lns: []net.Listener{l4}}
	switch l6, err := f.Listen(ctx, net.JoinHostPort("::1", p)); {
	case err == nil:
		fw.lns = append(fw.lns, l6)
	case errors.Is(err, syscall.EADDRINUSE):
		// Another program has it there: localhost would reach either.
		fw.close()
		return nil, err
	}
	for _, l := range fw.lns {
		go f.serve(ctx, l, port, fw)
	}
	return fw, nil
}

// serve relays each connection to a listener to the port in the VM.
func (f *Forwarder) serve(ctx context.Context, l net.Listener, port int, fw *forward) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		select {
		case f.relays <- struct{}{}:
		default:
			_ = conn.Close() // too many at once
			continue
		}
		f.mu.Lock()
		host := fw.host
		f.mu.Unlock()
		go func() {
			defer func() { <-f.relays }()
			f.Relay(ctx, conn, host, port)
		}()
	}
}

func (fw *forward) close() {
	for _, l := range fw.lns {
		_ = l.Close()
	}
}

func (f *Forwarder) closeAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for port, fw := range f.ports {
		fw.close()
		delete(f.ports, port)
	}
	clear(f.skipped)
}

// listeners reads ports.sh's report, /proc/net/tcp's local addresses
// (ADDRESS:PORT in hex), as the address in the VM each port's
// connections go to: 127.0.0.1 for a server on all IPv4 addresses or
// that one, ::1 for one on all IPv6 addresses or that one, another
// loopback address for one there alone. Servers on other addresses
// alone aren't forwarded, nor are lines that aren't addresses.
func listeners(report []string) map[int]string {
	found := map[int][]netip.Addr{}
	for _, line := range report {
		a, p, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || len(p) != 4 {
			continue
		}
		port, err := strconv.ParseUint(p, 16, 16)
		ip, iok := procAddr(a)
		if err != nil || port == 0 || !iok {
			continue
		}
		if to, ok := dialAddr(ip); ok {
			found[int(port)] = append(found[int(port)], to)
		}
	}
	out := map[int]string{}
	v4, v6 := netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback()
	for port, addrs := range found {
		switch {
		case slices.Contains(addrs, v4):
			out[port] = v4.String()
		case slices.Contains(addrs, v6):
			out[port] = v6.String()
		default:
			slices.SortFunc(addrs, func(a, b netip.Addr) int { return a.Compare(b) })
			out[port] = addrs[0].String()
		}
	}
	return out
}

// procAddr reads an address as /proc/net/tcp has it: 4 or 16 bytes in
// hex, each 32-bit word in the host's order (little-endian on devvm's).
func procAddr(h string) (netip.Addr, bool) {
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 4 && len(b) != 16 {
		return netip.Addr{}, false
	}
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	a, ok := netip.AddrFromSlice(b)
	return a.Unmap(), ok
}

// dialAddr is where in the VM a forwarded connection to a server on a goes:
// its loopback alone.
func dialAddr(a netip.Addr) (netip.Addr, bool) {
	switch {
	case a == netip.IPv4Unspecified():
		return netip.MustParseAddr("127.0.0.1"), true
	case a == netip.IPv6Unspecified():
		return netip.IPv6Loopback(), true
	case a.IsLoopback():
		return a, true
	}
	return netip.Addr{}, false
}

// readLifeline returns what a lifeline holds, "" when it's gone.
func readLifeline(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // smolvm's own file for the machine
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// lifelineHolds reports whether the lifeline still holds life and its
// process (the first line) runs.
func lifelineHolds(path, life string) bool {
	cur, err := readLifeline(path)
	if err != nil || cur != life {
		return false
	}
	first, _, _ := strings.Cut(cur, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	return err == nil && pid > 0 && alive(pid)
}

// listenLoopback listens on a host loopback address only where the port is
// free on every address (platform.ListenExclusive). A port in use only by
// connections that have ended (TIME_WAIT: nothing answers on it) is taken
// as Go takes any.
func listenLoopback(ctx context.Context, addr string) (net.Listener, error) {
	l, err := platform.ListenExclusive(ctx, "tcp", addr)
	if err == nil || !errors.Is(err, syscall.EADDRINUSE) {
		return l, err
	}
	d := net.Dialer{Timeout: 300 * time.Millisecond}
	if c, derr := d.DialContext(ctx, "tcp", addr); derr == nil {
		_ = c.Close()
		return nil, err // a program of the host's listens there
	} else if !errors.Is(derr, syscall.ECONNREFUSED) {
		return nil, err
	}
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", addr)
}

// sshForward is how the forwarder reaches a VM: its sshd's port on the
// host's loopback, the VM's user and key, its pinned host key, and the
// master's control socket.
type sshForward struct {
	vm, user, key, knownHosts, control string
	port                               int
}

// connArgs are ssh's options for reaching the VM: no configuration of the
// user's, the VM's key alone and no agent, its host key pinned, and
// nothing forwarded either way.
func (o sshForward) connArgs() []string {
	return []string{
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "IdentityFile=" + o.key,
		"-o", "PreferredAuthentications=publickey", "-o", "AddKeysToAgent=no",
		"-o", "UserKnownHostsFile=" + o.knownHosts, "-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "HostKeyAlias=" + o.vm, "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no",
		"-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "Tunnel=no", "-o", "PermitRemoteOpen=none",
		"-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "ProxyCommand=none", "-o", "ProxyJump=none",
		"-o", "ConnectTimeout=10", "-o", "LogLevel=ERROR",
		"-p", strconv.Itoa(o.port), "-l", o.user, "127.0.0.1",
	}
}

// masterArgs are ssh's arguments for the master connection, ports.sh run
// in its session.
func (o sshForward) masterArgs() []string {
	return slices.Concat(
		[]string{"-F", "none", "-M", "-S", o.control, "-T", "-o", "ControlPersist=no", "-o", "ServerAliveInterval=10", "-o", "ServerAliveCountMax=3"},
		o.connArgs(), []string{"sh -s"},
	)
}

// relayArgs are ssh's arguments for one forwarded connection: a channel of
// the master's to host:port in the VM. Without the master, ssh connects
// on its own: to the VM, as the master does, never anywhere else.
func (o sshForward) relayArgs(host string, port int) []string {
	return slices.Concat(
		[]string{"-F", "none", "-S", o.control, "-o", "ControlMaster=no", "-W", net.JoinHostPort(host, strconv.Itoa(port))},
		o.connArgs(),
	)
}

// watch starts the master, ports.sh fed to its session.
func (o sshForward) watch(script []byte, log io.Writer) func(ctx context.Context) (io.ReadCloser, func() error, error) {
	return func(ctx context.Context) (io.ReadCloser, func() error, error) {
		if err := os.Remove(o.control); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
		cmd := exec.CommandContext(ctx, "ssh", o.masterArgs()...) //nolint:gosec // devvm's own arguments
		cmd.Stdin, cmd.Stderr = bytes.NewReader(script), log
		out, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, err
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		return out, cmd.Wait, nil
	}
}

// relay carries conn through a channel of the master's.
func (o sshForward) relay(ctx context.Context, conn net.Conn, host string, port int) {
	defer func() { _ = conn.Close() }()
	cmd := exec.CommandContext(ctx, "ssh", o.relayArgs(host, port)...) //nolint:gosec // devvm's own arguments
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}
	go func() {
		_, _ = io.Copy(in, conn)
		_ = in.Close()
	}()
	_, _ = io.Copy(conn, out)
	_ = conn.Close()
	_ = cmd.Wait()
}

// ForwardMain runs a VM's forwarder (devvm _smolvm-ports) with its
// arguments: ports.sh is script, reserved the ports devvm records for VMs,
// and its lines go to log.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func() (map[int]bool, error), log io.Writer) error {
	fs := flag.NewFlagSet("_smolvm-ports", flag.ContinueOnError)
	fs.SetOutput(log)
	var o sshForward
	var lifeline, reserve string
	fs.StringVar(&o.vm, "vm", "", "the VM")
	fs.StringVar(&lifeline, "lifeline", "", "the machine's agent.pid: exit when it changes or its process is gone")
	fs.IntVar(&o.port, "port", 0, "the host loopback port the VM's sshd is forwarded from")
	fs.StringVar(&o.user, "user", "", "the VM's user")
	fs.StringVar(&o.key, "key", "", "the VM's own key")
	fs.StringVar(&o.knownHosts, "known-hosts", "", "the VM's host key, as known_hosts has it, under the VM's name")
	fs.StringVar(&o.control, "control", "", "the master's control socket")
	fs.StringVar(&reserve, "reserve", "", "host ports never forwarded, comma-separated")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if o.vm == "" || lifeline == "" || o.port <= 0 || o.user == "" || o.key == "" || o.knownHosts == "" || o.control == "" {
		return errors.New("-vm, -lifeline, -port, -user, -key, -known-hosts and -control are needed")
	}
	own := map[int]bool{}
	for p := range strings.SplitSeq(reserve, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			own[n] = true
		}
	}
	log = newLogLimit(log, time.Now)
	f := &Forwarder{
		VM: o.vm, Lifeline: lifeline, Log: log, Tick: 2 * time.Second, Gap: 500 * time.Millisecond,
		Reserved: func() (map[int]bool, error) {
			out, err := reserved()
			if out == nil {
				out = map[int]bool{}
			}
			for p := range own {
				out[p] = true
			}
			return out, err
		},
		Watch: o.watch(script, log), Relay: o.relay, Listen: listenLoopback,
	}
	return f.Run(ctx)
}
