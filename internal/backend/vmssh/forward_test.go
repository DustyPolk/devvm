package vmssh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/platform"
)

// ports.sh's report as the forwarder reads it: each port to the address
// in the VM its connections go to, loopback alone.
func TestListeners(t *testing.T) {
	got := listeners([]string{
		"0100007F:0BB8",                            // 127.0.0.1:3000
		"00000000:1F90",                            // 0.0.0.0:8080
		"00000000000000000000000001000000:1F90",    // [::1]:8080: IPv4 first
		"00000000000000000000000001000000:1435",    // [::1]:5173 alone
		"00000000000000000000000000000000:2328",    // [::]:9000
		"0000000000000000FFFF00000100007F:2329",    // [::ffff:127.0.0.1]:9001
		"0200007F:232A",                            // 127.0.0.2:9002
		"0100A8C0:232B",                            // 192.168.0.1:9003: not loopback
		"0000000000000000FFFF00000100A8C0:232C",    // [::ffff:192.168.0.1]:9004
		"0100007F:0000", "zz:0BB8", "0100007F", "", // not addresses
		"0100007F:00016", "01007F:0BB8",
		// links' listeners, in 127.77.0.0/16: never forwarded
		"01004D7F:1538",                         // 127.77.0.1:5432 alone
		"0000000000000000FFFF0000FEFF4D7F:1539", // [::ffff:127.77.255.254]:5433
		"02004D7F:0050",                         // 127.77.0.2:80
		"01004D7F:0BB8",                         // 127.77.0.1:3000, beside the VM's own 127.0.0.1:3000
		"01004E7F:153A",                         // 127.78.0.1:5434: not a link's
		"01004C7F:153B",                         // 127.76.0.1:5435: not a link's
	})
	want := map[int]string{
		3000: "127.0.0.1", 8080: "127.0.0.1", 5173: "::1", 9000: "::1", 9001: "127.0.0.1", 9002: "127.0.0.2",
		5434: "127.78.0.1", 5435: "127.76.0.1",
	}
	if !maps.Equal(got, want) {
		t.Errorf("listeners = %v, want %v", got, want)
	}
}

// freePorts returns n ports free on the host's loopback now.
func freePorts(t *testing.T, n int) []int {
	t.Helper()
	var out []int
	for len(out) < n {
		l, err := net.Listen("tcp", "127.0.0.1:0") //nolint:noctx // a test's moment
		if err != nil {
			t.Fatal(err)
		}
		p := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if p >= minForwardPort && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// procLine is a listener on port as /proc/net/tcp has it.
func procLine(addr string, port int) string { return fmt.Sprintf("%s:%04X", addr, port) }

// fakeVM is a VM as the forwarder sees it: its reports, written by the
// test, one connection at a time.
type fakeVM struct {
	mu      sync.Mutex
	reports *io.PipeWriter
	watches int
	ready   chan struct{}
}

func (v *fakeVM) watch(ctx context.Context) (io.ReadCloser, func() error, error) {
	r, w := io.Pipe()
	v.mu.Lock()
	v.reports = w
	v.watches++
	v.mu.Unlock()
	go func() { <-ctx.Done(); _ = w.CloseWithError(ctx.Err()) }()
	v.ready <- struct{}{}
	return r, func() error { return nil }, nil
}

func (v *fakeVM) report(t *testing.T, lines ...string) {
	t.Helper()
	v.mu.Lock()
	w := v.reports
	v.mu.Unlock()
	if _, err := io.WriteString(w, strings.Join(append(lines, "."), "\n")+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (v *fakeVM) drop() {
	v.mu.Lock()
	_ = v.reports.Close()
	v.mu.Unlock()
}

// reach connects to a host loopback port and returns what comes back.
func reach(t *testing.T, addr string) string {
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(t.Context(), "tcp", addr)
	if err != nil {
		return "refused"
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	line, _ := bufio.NewReader(c).ReadString('\n')
	return strings.TrimSpace(line)
}

// eventually waits for addr to answer want.
func eventually(t *testing.T, addr, want string) {
	t.Helper()
	var got string
	for range 100 {
		if got = reach(t, addr); got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("%s: %q, want %q", addr, got, want)
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(b)
}

// echo is a relay that answers where the connection would go.
func echo(_ context.Context, conn net.Conn, h string, p int) {
	_, _ = fmt.Fprintf(conn, "to %s\n", net.JoinHostPort(h, strconv.Itoa(p)))
	_ = conn.Close()
}

// The forwarder listens on the host's loopback for what the VM reports,
// from 1024 up, never on a reserved port or one the host has on any
// address; forwards go once the VM's listener does, and all of them when
// the connection ends, until it's made again; it ends once the VM no
// longer runs as it did.
func TestForwarder(t *testing.T) {
	ports := freePorts(t, 5)
	open, v6only, reserved, hosts, gone := ports[0], ports[1], ports[2], ports[3], ports[4]
	// The host's own program, on every address.
	host, err := net.Listen("tcp", ":"+strconv.Itoa(hosts)) //nolint:noctx // the test's
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = host.Close() }()
	var alive atomic.Bool
	alive.Store(true)
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	var log bytes.Buffer
	var logMu sync.Mutex
	f := &Forwarder{
		VM: "devvm-f", Tick: 10 * time.Millisecond, Every: 10 * time.Millisecond, Log: lockedWriter{&logMu, &log},
		Alive:    func(context.Context) bool { return alive.Load() },
		Reserved: func() (map[int]bool, error) { return map[int]bool{reserved: true}, nil },
		Watch:    vm.watch, Relay: echo, Listen: platform.ListenLoopback,
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- f.Run(ctx) }()
	<-vm.ready
	vm.report(t,
		procLine("0100007F", open), procLine("00000000000000000000000001000000", v6only),
		procLine("00000000", reserved), procLine("00000000", hosts), procLine("00000000", gone),
		procLine("00000000", 80), procLine("0100A8C0", 9999),
	)
	at := func(p int) string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(p)) }
	eventually(t, at(open), "to "+at(open))
	eventually(t, net.JoinHostPort("::1", strconv.Itoa(open)), "to "+at(open))
	eventually(t, at(v6only), "to "+net.JoinHostPort("::1", strconv.Itoa(v6only)))
	eventually(t, at(gone), "to "+at(gone))
	if got := reach(t, at(reserved)); got != "refused" {
		t.Errorf("reserved port: %q", got)
	}
	if got := reach(t, at(hosts)); strings.HasPrefix(got, "to ") {
		t.Errorf("the host's port taken: %q", got)
	}
	// The VM's listener goes: so does its forward.
	vm.report(t, procLine("0100007F", open))
	eventually(t, at(gone), "refused")
	eventually(t, at(open), "to "+at(open))
	// The connection ends: every forward goes, then it's made again.
	vm.drop()
	<-vm.ready
	eventually(t, at(open), "refused")
	vm.report(t, procLine("00000000", open))
	eventually(t, at(open), "to "+at(open))
	// The VM no longer runs as it did (started again): it ends.
	alive.Store(false)
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Error("still running with its VM gone")
	}
	cancel()
	eventually(t, at(open), "refused")
	logMu.Lock()
	defer logMu.Unlock()
	for _, want := range []string{"forwarding devvm-f's ports", fmt.Sprintf("port %d: not forwarded", hosts), fmt.Sprintf("port %d: forwarded", open), "devvm-f stopped"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}
}

// A VM that isn't running gets no forwarder; one whose connection ends
// because it stopped ends it at once, without waiting for Every.
func TestForwarderAliveOnEnd(t *testing.T) {
	f := &Forwarder{VM: "devvm-f", Alive: func(context.Context) bool { return false }}
	if err := f.Run(t.Context()); err == nil {
		t.Error("ran for a VM that isn't running")
	}
	var calls atomic.Int32
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	f = &Forwarder{
		VM: "devvm-f", Tick: time.Hour, Every: time.Hour, Log: io.Discard,
		Alive:    func(context.Context) bool { return calls.Add(1) <= 2 },
		Reserved: func() (map[int]bool, error) { return nil, nil },
		Watch:    vm.watch, Listen: platform.ListenLoopback,
	}
	done := make(chan error, 1)
	go func() { done <- f.Run(t.Context()) }()
	<-vm.ready
	vm.drop()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Error("still running with its connection ended and its VM gone")
	}
}

// With Only (listed ports), the forwarder forwards those alone, below
// 1024 too, and nothing else the VM listens on.
func TestForwarderOnly(t *testing.T) {
	ports := freePorts(t, 3)
	listed, other, reserved := ports[0], ports[1], ports[2]
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	f := &Forwarder{
		VM: "devvm-f", Tick: 10 * time.Millisecond, Every: time.Hour, Log: io.Discard,
		Only:     []backend.PortRange{{From: listed, To: listed}, {From: reserved, To: reserved}, {From: 80, To: 80}},
		Alive:    func(context.Context) bool { return true },
		Reserved: func() (map[int]bool, error) { return map[int]bool{reserved: true}, nil },
		Watch:    vm.watch, Relay: echo, Listen: platform.ListenLoopback,
		ports: map[int]*forward{}, skipped: map[int]bool{},
	}
	if !f.wanted(80) || f.wanted(other) || !f.wanted(listed) {
		t.Error("wanted: not the listed ports alone")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = f.Run(ctx) }()
	<-vm.ready
	vm.report(t, procLine("00000000", listed), procLine("00000000", other), procLine("00000000", reserved))
	at := func(p int) string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(p)) }
	eventually(t, at(listed), "to "+at(listed))
	if got := reach(t, at(other)); got != "refused" {
		t.Errorf("an unlisted port: %q", got)
	}
	if got := reach(t, at(reserved)); got != "refused" {
		t.Errorf("a listed port devvm reserves: %q", got)
	}
}

// A VM that reports faster than Gap waits: reports are applied Gap apart.
func TestForwarderGap(t *testing.T) {
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	var mu sync.Mutex
	var applied []time.Time
	gap := 100 * time.Millisecond
	f := &Forwarder{
		VM: "devvm-f", Tick: 10 * time.Millisecond, Every: time.Hour, Gap: gap, Log: io.Discard,
		Alive: func(context.Context) bool { return true },
		Reserved: func() (map[int]bool, error) {
			mu.Lock()
			defer mu.Unlock()
			applied = append(applied, time.Now())
			return nil, nil
		},
		Watch: vm.watch, Listen: platform.ListenLoopback,
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() { _ = f.Run(ctx) }()
	<-vm.ready
	for range 5 {
		vm.report(t)
	}
	time.Sleep(gap)
	mu.Lock()
	defer mu.Unlock()
	if len(applied) < 4 {
		t.Fatalf("%d reports applied", len(applied))
	}
	for i := 1; i < len(applied); i++ {
		if d := applied[i].Sub(applied[i-1]); d < gap-10*time.Millisecond {
			t.Errorf("reports %d and %d applied %v apart", i-1, i, d)
		}
	}
}

// What the VM makes the forwarder log is limited: a burst, then one write
// per logEvery, those dropped counted; control characters shown as '?',
// long writes cut.
func TestLogLimit(t *testing.T) {
	var b bytes.Buffer
	now := time.Unix(1000, 0)
	l := newLogLimit(&b, func() time.Time { return now })
	for range logBurst + 50 {
		_, _ = l.Write([]byte("x\n"))
	}
	if got := strings.Count(b.String(), "x\n"); got != logBurst {
		t.Errorf("%d writes logged, want %d", got, logBurst)
	}
	b.Reset()
	now = now.Add(2 * logEvery)
	_, _ = l.Write([]byte("a\x1b[2Jb\r\n"))
	_, _ = l.Write([]byte(strings.Repeat("y", logWriteMax+10)))
	_, _ = l.Write([]byte("dropped\n"))
	want := "devvm ports: 50 writes not logged\na?[2Jb?\n" + strings.Repeat("y", logWriteMax) + "…\n"
	if b.String() != want {
		t.Errorf("log:\n%q\nwant\n%q", b.String(), want)
	}
}

// What a VM prints is kept up to the cap.
func TestCapped(t *testing.T) {
	c := &Capped{Max: 4}
	if n, err := c.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatal(n, err)
	}
	if n, _ := c.Write([]byte("defg")); n != 4 || c.String() != "abcd" {
		t.Errorf("kept %q", c.String())
	}
}

// While devvm's ports can't be read, nothing new is forwarded.
func TestForwarderReservedUnknown(t *testing.T) {
	p := freePorts(t, 1)[0]
	var log bytes.Buffer
	f := &Forwarder{
		VM: "devvm-f", Log: &log, ports: map[int]*forward{}, skipped: map[int]bool{},
		Reserved: func() (map[int]bool, error) { return nil, errors.New("unreadable") },
		Listen:   platform.ListenLoopback,
	}
	f.apply(t.Context(), map[int]string{p: "127.0.0.1"})
	if len(f.ports) != 0 || !strings.Contains(log.String(), "no new ports forwarded") {
		t.Errorf("forwarded %v; log %q", f.ports, log.String())
	}
}

// The master asks for nothing to be forwarded, reads no configuration of
// the user's and offers the VM's key alone; a relay is a channel of it,
// and without it connects to the VM the same way, never elsewhere: on a
// host port, or through the VM's ProxyCommand.
func TestForwarderSSH(t *testing.T) {
	o := SSH{VM: "devvm-v", User: "u", Key: "/k", KnownHosts: "/kh", Control: "/c.ctl", Port: 61022}
	conn := []string{
		"IdentitiesOnly=yes", "IdentityAgent=none", "IdentityFile=/k", "UserKnownHostsFile=/kh",
		"HostKeyAlias=devvm-v", "StrictHostKeyChecking=yes", "ForwardAgent=no", "ForwardX11=no", "Tunnel=no",
		"PermitRemoteOpen=none", "ClearAllForwardings=yes",
	}
	p := o
	p.Port, p.Proxy = 0, "'/devvm' '_docker-connect' 'devvm-v'"
	for name, tc := range map[string]struct {
		args []string
		want []string
	}{
		"master":       {o.masterArgs(), append([]string{"-F none -M -S /c.ctl -T ", "ProxyCommand=none -p 61022 -l u 127.0.0.1 sh -s"}, conn...)},
		"relay":        {o.relayArgs("::1", 5173), append([]string{"-F none -S /c.ctl -o ControlMaster=no -W [::1]:5173 ", "ProxyCommand=none -p 61022 -l u 127.0.0.1"}, conn...)},
		"proxy master": {p.masterArgs(), append([]string{"-o ProxyCommand='/devvm' '_docker-connect' 'devvm-v' -l u devvm-v sh -s"}, conn...)},
		"proxy relay":  {p.relayArgs("127.0.0.1", 80), append([]string{"-W 127.0.0.1:80 ", "-o ProxyCommand='/devvm' '_docker-connect' 'devvm-v' -l u devvm-v"}, conn...)},
	} {
		got := strings.Join(tc.args, " ")
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Errorf("%s lacks %q: %s", name, want, got)
			}
		}
		for _, a := range tc.args {
			if a == "-R" || a == "-L" || a == "-D" || a == "-A" || a == "-X" || a == "-Y" {
				t.Errorf("%s asks for %s", name, a)
			}
		}
	}
	if got := o.relayArgs("::1", 5173); got[len(got)-1] != "127.0.0.1" {
		t.Errorf("relay runs a command: %q", got)
	}
	if got := p.relayArgs("::1", 5173); got[len(got)-1] != "devvm-v" {
		t.Errorf("proxy relay runs a command: %q", got)
	}
}

// A session forwards the agent asked for and nothing else, reads no
// configuration of the user's, and offers the VM's key alone.
func TestSessionArgs(t *testing.T) {
	o := SSH{VM: "devvm-v", User: "u", Key: "/k", KnownHosts: "/kh", Proxy: "'/devvm' 'x'"}
	for agent, want := range map[string][]string{
		"":        {"IdentityAgent=none", "ForwardAgent=no"},
		AgentUser: {"IdentityAgent=SSH_AUTH_SOCK", "ForwardAgent=yes"},
		"/a.sock": {`IdentityAgent="/a.sock"`, `ForwardAgent="/a.sock"`},
	} {
		args := o.SessionArgs(agent, true, "cd -- '/p' && exec bash -l")
		got := strings.Join(args, " ")
		if !strings.HasPrefix(got, "-F none -t -o ControlMaster=no -o ControlPath=none ") || args[len(args)-1] != "cd -- '/p' && exec bash -l" ||
			args[len(args)-2] != "devvm-v" || !strings.Contains(got, "ClearAllForwardings=yes") || !strings.Contains(got, "IdentitiesOnly=yes") {
			t.Errorf("agent %q: %q", agent, args)
		}
		for _, w := range want {
			if !slices.Contains(args, w) {
				t.Errorf("agent %q lacks %s: %q", agent, w, args)
			}
		}
	}
	if args := o.SessionArgs("", false, "x"); args[2] != "-T" {
		t.Errorf("no terminal: %q", args)
	}
}

func newFlagSet(fl *Flags) *flag.FlagSet {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fl.Register(fs)
	return fs
}

// The forwarder's flags: each backend's command adds its own, and reaches
// its VM one way or another; listed ports are read as devvm ports lists
// them.
func TestFlags(t *testing.T) {
	var fl Flags
	fs := newFlagSet(&fl)
	if err := fs.Parse([]string{"-vm", "v", "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-reserve", "1,2", "-only", "80,8000-8002"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fl.Forwarder(nil, func(string) (map[int]bool, error) { return nil, nil }, io.Discard); err == nil {
		t.Error("no way to the VM: taken")
	}
	fl.Proxy = "'x'"
	f, err := fl.Forwarder(nil, func(vm string) (map[int]bool, error) { return map[int]bool{3: vm == "v"}, nil }, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.Only, []backend.PortRange{{From: 80, To: 80}, {From: 8000, To: 8002}}) {
		t.Errorf("Only = %v", f.Only)
	}
	if r, err := f.Reserved(); err != nil || !maps.Equal(r, map[int]bool{1: true, 2: true, 3: true}) {
		t.Errorf("Reserved = %v, %v", r, err)
	}
	for _, bad := range []string{"x", "0", "9-1", "70000", ""} {
		if _, err := ParseRanges(bad); err == nil {
			t.Errorf("ParseRanges(%q): taken", bad)
		}
	}
	if FormatRanges(f.Only) != "80,8000-8002" {
		t.Error(FormatRanges(f.Only))
	}
}

// A forwarder that doesn't end on SIGTERM is killed: Stop returns with it
// gone, so a start that follows finds its ports free.
func TestForwardsStopKills(t *testing.T) {
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	p, err := platform.StartDetachedProcess("/bin/sh", []string{"-c", `trap "" TERM; echo ready; while :; do sleep 1; done`}, out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Kill() }()
	ended := make(chan struct{})
	go func() { _, _ = p.Wait(); close(ended) }()
	for range 100 {
		if b, _ := os.ReadFile(out.Name()); strings.Contains(string(b), "ready") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	f := Forwards{Dir: dir, Cmd: "_x-ports", Args: func(int) ([]string, error) {
		select {
		case <-ended:
			return nil, errors.New("gone")
		default:
			return []string{"/devvm", "_x-ports", "-vm", "v"}, nil
		}
	}}
	if err := os.WriteFile(filepath.Join(dir, "v.ports.pid"), []byte(strconv.Itoa(p.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.Stop("v")
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Error("still running after Stop")
	}
}

// Forwards: a VM's forwarder runs devvm's hidden command with the
// backend's flags and the forwarder's own, its host key pinned in a file
// of its own; it's found by its pid and arguments, stopped, and its files
// deleted.
func TestForwards(t *testing.T) {
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	var args []string
	running := false
	f := Forwards{
		Dir: filepath.Join(short, "vms"), Run: filepath.Join(short, "run"), Prefix: "x", Cmd: "_x-ports", Self: "/devvm",
		Detach: func(path string, a []string, out *os.File) (int, error) {
			args = append([]string{path}, a...)
			_, err := fmt.Fprintf(out, "devvm ports: forwarding v's ports\n")
			return os.Getpid(), err
		},
		Args: func(int) ([]string, error) {
			if !running {
				return nil, errors.New("gone")
			}
			return []string{"/devvm", "_x-ports", "-vm", "v"}, nil
		},
		Tick: time.Millisecond,
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	spec := Spec{User: "u", Key: "/k", HostKey: "ssh-ed25519 AAAA", Reserve: []int{0, 22}, Only: []backend.PortRange{{From: 80, To: 81}}, Args: []string{"-a", "b"}}
	if err := f.Start("v", spec); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/devvm", "_x-ports", "-vm", "v", "-a", "b", "-user", "u", "-key", "/k", "-known-hosts", f.KnownHosts("v"),
		"-control", f.Control("v"), "-reserve", "22", "-only", "80-81",
	}
	if !slices.Equal(args, want) || !strings.HasPrefix(f.Control("v"), filepath.Join(short, "run", "x-")) {
		t.Errorf("forwarder:\n%q\nwant\n%q", args, want)
	}
	if b, err := os.ReadFile(f.KnownHosts("v")); err != nil || string(b) != "v ssh-ed25519 AAAA\n" {
		t.Errorf("known_hosts: %q, %v", b, err)
	}
	if f.Running("v") {
		t.Error("found with other arguments")
	}
	running = true
	if !f.Running("v") || f.Running("w") {
		t.Error("not found by its arguments")
	}
	running = false
	// Its lock: held by one at a time, waited for until the context ends;
	// another VM's is its own.
	unlock, err := f.Lock(t.Context(), "v")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if _, err := f.Lock(ctx, "v"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("locked twice: %v", err)
	}
	if unlockW, err := f.Lock(t.Context(), "w"); err != nil {
		t.Errorf("another VM's: %v", err)
	} else {
		unlockW()
	}
	unlock()
	if unlock, err = f.Lock(t.Context(), "v"); err != nil {
		t.Errorf("once unlocked: %v", err)
	} else {
		unlock()
	}
	if err := f.Remove("v"); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".ports.pid", ".ports.known_hosts", ".ports.log", ".ports.lock"} {
		if _, err := os.Stat(filepath.Join(f.Dir, "v"+ext)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", ext, err)
		}
	}
	// One that never says it forwards: an error, left running.
	f.Detach = func(string, []string, *os.File) (int, error) { return os.Getpid(), nil }
	running = true
	f.Tick = time.Microsecond
	if err := f.Start("v", spec); err == nil {
		t.Error("a forwarder that doesn't forward: no error")
	}
}
