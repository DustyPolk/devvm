package smolvm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
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
	})
	want := map[int]string{3000: "127.0.0.1", 8080: "127.0.0.1", 5173: "::1", 9000: "::1", 9001: "127.0.0.1", 9002: "127.0.0.2"}
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

// The forwarder listens on the host's loopback for what the VM reports,
// from 1024 up, never on a reserved port or one the host has on any
// address; forwards go once the VM's listener does, and all of them when
// the connection ends, until it's made again.
func TestForwarder(t *testing.T) {
	ports := freePorts(t, 5)
	open, v6only, reserved, hosts, gone := ports[0], ports[1], ports[2], ports[3], ports[4]
	// The host's own program, on every address.
	host, err := net.Listen("tcp", ":"+strconv.Itoa(hosts)) //nolint:noctx // the test's
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = host.Close() }()
	life := filepath.Join(t.TempDir(), "agent.pid")
	if err := os.WriteFile(life, []byte(strconv.Itoa(os.Getpid())+"\n123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	var log bytes.Buffer
	var logMu sync.Mutex
	f := &Forwarder{
		VM: "devvm-f", Lifeline: life, Tick: 10 * time.Millisecond, Log: lockedWriter{&logMu, &log},
		Reserved: func() (map[int]bool, error) { return map[int]bool{reserved: true}, nil },
		Watch:    vm.watch,
		Relay: func(_ context.Context, conn net.Conn, h string, p int) {
			_, _ = fmt.Fprintf(conn, "to %s\n", net.JoinHostPort(h, strconv.Itoa(p)))
			_ = conn.Close()
		},
		Listen: platform.ListenLoopback,
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
	vm.mu.Lock()
	_ = vm.reports.Close()
	vm.mu.Unlock()
	<-vm.ready
	eventually(t, at(open), "refused")
	vm.report(t, procLine("00000000", open))
	eventually(t, at(open), "to "+at(open))
	// The lifeline changes (the VM started again): it ends.
	if err := os.WriteFile(life, []byte(strconv.Itoa(os.Getpid())+"\n456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Error("still running with its lifeline changed")
	}
	cancel()
	eventually(t, at(open), "refused")
	logMu.Lock()
	defer logMu.Unlock()
	for _, want := range []string{"forwarding devvm-f's ports", fmt.Sprintf("port %d: not forwarded", hosts), fmt.Sprintf("port %d: forwarded", open)} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}
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

// A VM that reports faster than Gap waits: reports are applied Gap apart.
func TestForwarderGap(t *testing.T) {
	life := filepath.Join(t.TempDir(), "agent.pid")
	if err := os.WriteFile(life, []byte(strconv.Itoa(os.Getpid())+"\n1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	vm := &fakeVM{ready: make(chan struct{}, 4)}
	var mu sync.Mutex
	var applied []time.Time
	gap := 100 * time.Millisecond
	f := &Forwarder{
		VM: "devvm-f", Lifeline: life, Tick: 10 * time.Millisecond, Gap: gap, Log: io.Discard,
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
	c := &capped{max: 4}
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
// and without it connects to the VM the same way, never elsewhere.
func TestForwarderSSH(t *testing.T) {
	o := sshForward{vm: "devvm-v", user: "u", key: "/k", knownHosts: "/kh", control: "/c.ctl", port: 61022}
	conn := []string{
		"IdentitiesOnly=yes", "IdentityAgent=none", "IdentityFile=/k", "UserKnownHostsFile=/kh",
		"HostKeyAlias=devvm-v", "StrictHostKeyChecking=yes", "ForwardAgent=no", "ForwardX11=no", "Tunnel=no",
		"PermitRemoteOpen=none", "ClearAllForwardings=yes", "ProxyCommand=none",
	}
	for name, tc := range map[string]struct {
		args []string
		want []string
	}{
		"master": {o.masterArgs(), append([]string{"-F none -M -S /c.ctl -T ", " -p 61022 -l u 127.0.0.1 sh -s"}, conn...)},
		"relay":  {o.relayArgs("::1", 5173), append([]string{"-F none -S /c.ctl -o ControlMaster=no -W [::1]:5173 ", " -p 61022 -l u 127.0.0.1"}, conn...)},
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
}

// A VM that forwards every port gets none of smolvm's, and its forwarder
// once its sshd runs: its host key pinned, its ssh's, proxy's and
// decider's ports reserved; stopping or deleting the VM stops it.
func TestStartForwarder(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-a-0123456789abcd"
	s.image(t, "0123456789ab")
	spec := backend.VMSpec{Name: vm, Ports: backend.Ports{All: true}}
	if err := s.CreateVM(t.Context(), "0123456789ab", spec); err != nil {
		t.Fatal(err)
	}
	if got := s.calls(t); len(got) != 1 || strings.Contains(got[0], " -p ") {
		t.Errorf("create: %q", got)
	}
	// A short folder for its control socket: a test's temporary one can
	// be too long for one.
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	s.Forwards.Dirs.Data = short
	var args []string
	s.Forwards.Detach = func(path string, a []string, out *os.File) (int, error) {
		args = append([]string{path}, a...)
		_, err := fmt.Fprintf(out, "devvm ports: forwarding %s's ports\n", vm)
		return os.Getpid(), err
	}
	running := false
	s.Forwards.Args = func(int) ([]string, error) {
		if !running {
			return nil, errors.New("gone")
		}
		return []string{"/devvm", forwarderCmd, "-vm", vm}, nil
	}
	var warned []string
	s.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	c := backend.Confinement{SSH: true, Net: backend.NetDirect, ProxyPort: 30001, Decider: "127.0.0.1:30002"}
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if len(warned) != 0 {
		t.Errorf("warned %q", warned)
	}
	r, _ := s.readRecord(vm)
	want := []string{
		"/devvm", "_smolvm-ports", "-vm", vm, "-lifeline", filepath.Join(s.stub, "smolvm-data", vm, "agent.pid"),
		"-port", "61022", "-user", "u", "-key", s.sshKey(vm), "-known-hosts", s.Forwards.file(vm, ".ports.known_hosts"),
		"-control", s.Forwards.control(vm), "-reserve", "61022,30001,30002",
	}
	if !r.PortsAll || !slices.Equal(args, want) {
		t.Errorf("forwarder:\n%q\nwant\n%q", args, want)
	}
	if b, err := os.ReadFile(s.Forwards.file(vm, ".ports.known_hosts")); err != nil || string(b) != vm+" ssh-ed25519 AAAAstub\n" {
		t.Errorf("known_hosts: %q, %v", b, err)
	}
	if !slices.Contains(s.calls(t), "smolvm machine exec --name "+vm+" -- cat /etc/ssh/ssh_host_ed25519_key.pub") {
		t.Errorf("host key not read through smolvm: %q", s.calls(t))
	}
	running = true
	if _, ok := s.Forwards.pid(vm); !ok {
		t.Error("forwarder not found")
	}
	running = false

	// Without sshd: warned, no forwarder.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.touch(t, "no-sshd")
	args = nil
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if args != nil || !slices.Contains(warned, "smolvm_ports_no_ssh vm="+vm) {
		t.Errorf("no sshd: forwarder %q, warned %q", args, warned)
	}
	if err := os.Remove(filepath.Join(s.stub, "no-sshd")); err != nil {
		t.Fatal(err)
	}
	// A forwarder that never says it forwards: warned, the VM runs on.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	warned = nil
	s.Forwards.Detach = func(string, []string, *os.File) (int, error) { return os.Getpid(), nil }
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(warned, []string{"smolvm_ports_failed log=" + s.Forwards.Log(vm) + " vm=" + vm}) {
		t.Errorf("warned %q", warned)
	}

	// Listed ports instead: smolvm's, and no forwarder.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.SetPorts(t.Context(), vm, backend.Ports{Ranges: []backend.PortRange{{From: 3000, To: 3000}}}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.readRecord(vm); r.PortsAll || !slices.Equal(s.calls(t), []string{"smolvm machine update --name " + vm + " -p 3000:3000"}) {
		t.Errorf("listed: %+v, %q", r, s.calls(t))
	}
	s.reset(t)
	if err := s.SetPorts(t.Context(), vm, backend.Ports{All: true}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.readRecord(vm); !r.PortsAll || !slices.Equal(s.calls(t), []string{"smolvm machine update --name " + vm + " --remove-port 3000:3000"}) {
		t.Errorf("all again: %+v, %q", r, s.calls(t))
	}
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".ports.pid", ".ports.known_hosts", ".ports.log"} {
		if _, err := os.Stat(s.Forwards.file(vm, ext)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", ext, err)
		}
	}
}
