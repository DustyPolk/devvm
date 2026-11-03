package lima

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/daemon"
)

// fakeForwarders makes l's forwarders processes of the test's (sleep),
// which say they forward at once, and returns each start's arguments.
func fakeForwarders(t *testing.T, l *Lima, vm string) *[][]string {
	t.Helper()
	var starts [][]string
	procs := map[int]bool{}
	l.Forwards.Detach = func(c daemon.Cmd) (int, error) {
		cmd := exec.CommandContext(t.Context(), "sleep", "30")
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		go func() { _ = cmd.Wait() }()
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		starts = append(starts, append([]string{c.Path}, c.Args...))
		procs[cmd.Process.Pid] = true
		return cmd.Process.Pid, daemon.WriteStatus(c.Status, cmd.Process.Pid, "")
	}
	l.Forwards.Args = func(pid int) ([]string, error) {
		if !procs[pid] {
			return nil, errors.New("not ours")
		}
		return []string{l.Forwards.Self, forwarderCmd, "-vm", vm}, nil
	}
	return &starts
}

// A project VM's start starts its forwarder once it runs: over Lima's ssh
// (its port, user and key), with the host key devvm pins, following the
// hostagent, never on its ssh's or proxy's port; its listed ports alone
// when it lists them, none and no forwarder when it forwards none. A
// build gets none. One that can't start is warned about, the VM running
// on, as does one without a ports record. Stopping the VM stops it;
// deleting it removes its files.
func TestStartForwarder(t *testing.T) {
	l, warned, stubs := stubLima(t)
	// Confined as on this host: macOS's sandbox-exec runs for real.
	l.OS = runtime.GOOS
	vm := "devvm-p-0123456789abcd"
	stubs.Write(t, "instances", vm+" Stopped\n")
	starts := fakeForwarders(t, l, vm)
	ctx := t.Context()
	key := func(context.Context) (string, error) { return "ssh-ed25519 AAAAkey", nil }
	c := backend.Confinement{Net: backend.NetProxy, ProxyPort: 30001, HostKey: key}
	// Without a ports record (CreateVM writes one): none, warned.
	if err := l.Start(ctx, vm, c); err != nil {
		t.Fatal(err)
	}
	if len(*starts) != 0 || !slices.Equal(warned.Names(), []string{"ports_forward_failed log=" + l.Forwards.Log(vm) + " vm=" + vm}) {
		t.Fatalf("no record: started %q, warned %q", *starts, warned.Names())
	}
	warned.Reset()
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if err := l.Forwards.write(vm, backend.Ports{All: true}); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(ctx, vm, c); err != nil {
		t.Fatal(err)
	}
	want := []string{
		l.Forwards.Self, "_lima-ports", "-vm", vm, "-lifeline", l.haPIDFile(vm), "-port", "60022",
		"-user", "stub", "-key", filepath.Join(l.Dirs.Lima, "_config", "user"), "-known-hosts", l.Forwards.KnownHosts(vm),
		"-control", l.Forwards.Control(vm), "-reserve", "60022,30001",
	}
	if len(*starts) != 1 || !slices.Equal((*starts)[0], want) {
		t.Fatalf("forwarder:\n%q\nwant\n%q", *starts, want)
	}
	if b, err := os.ReadFile(l.Forwards.KnownHosts(vm)); err != nil || string(b) != vm+" ssh-ed25519 AAAAkey\n" {
		t.Errorf("known_hosts: %q, %v", b, err)
	}
	if !strings.HasPrefix(filepath.Base(l.Forwards.Control(vm)), "lima-") || filepath.Dir(l.Forwards.Log(vm)) != l.Dirs.LimaVMs() {
		t.Errorf("its files: %s, %s", l.Forwards.Control(vm), l.Forwards.Log(vm))
	}
	if !l.Forwards.Running(vm) {
		t.Error("forwarder not running")
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if l.Forwards.Running(vm) {
		t.Error("forwarder running with its VM stopped")
	}

	// Listed ports: those Free leaves this start.
	listed := []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8010}}
	if err := l.SetPorts(ctx, vm, backend.Ports{Ranges: listed}, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	c.Free = func(_ context.Context, rs []backend.PortRange) []backend.PortRange {
		if !slices.Equal(rs, listed) {
			t.Errorf("Free asked of %v", rs)
		}
		return rs[1:]
	}
	if err := l.Start(ctx, vm, c); err != nil {
		t.Fatal(err)
	}
	if got := (*starts)[len(*starts)-1]; !slices.Equal(got[len(got)-2:], []string{"-only", "8000-8010"}) {
		t.Errorf("listed: %q", got)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}

	// None: no forwarder; a build: none either.
	if err := l.SetPorts(ctx, vm, backend.Ports{}, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	c.Free = nil
	n := len(*starts)
	if err := l.Start(ctx, vm, c); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPorts(ctx, vm, backend.Ports{All: true}, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(ctx, vm, backend.Confinement{Net: backend.NetInternet}); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if len(*starts) != n || len(warned.Names()) != 0 {
		t.Errorf("forwarders started %q, warned %q", (*starts)[n:], warned.Names())
	}

	// Its host key unknown: warned, the VM runs.
	c.HostKey = func(context.Context) (string, error) { return "", errors.New("no keys") }
	if err := l.Start(ctx, vm, c); err != nil {
		t.Fatal(err)
	}
	if len(*starts) != n || !slices.Equal(warned.Names(), []string{"ports_forward_failed log=" + l.Forwards.Log(vm) + " vm=" + vm}) {
		t.Errorf("no host key: started %q, warned %q", (*starts)[n:], warned.Names())
	}
	if err := l.Delete(ctx, vm); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{l.Forwards.file(vm), l.Forwards.KnownHosts(vm), l.Forwards.Log(vm)} {
		if _, err := os.Stat(f); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", f, err)
		}
	}
}

// A running VM's ports change at once: its forwarder starts again with
// the new ones, those Free leaves, the old one stopped first; none stops
// it. A stopped VM's change starts none.
func TestSetPortsRunning(t *testing.T) {
	l, warned, stubs := stubLima(t)
	vm := "devvm-p-0123456789abcd"
	stubs.Write(t, "instances", vm+" Stopped\n")
	starts := fakeForwarders(t, l, vm)
	ctx := t.Context()
	key := func(context.Context) (string, error) { return "ssh-ed25519 AAAAkey", nil }
	if err := l.Forwards.write(vm, backend.Ports{All: true}); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(ctx, vm, backend.Confinement{HostKey: key}); err != nil {
		t.Fatal(err)
	}
	running := false
	c := backend.Confinement{HostKey: key, ProxyPort: 30001, Free: func(_ context.Context, rs []backend.PortRange) []backend.PortRange {
		running = l.Forwards.Running(vm)
		return rs[:1]
	}}
	listed := backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8000}}}
	if err := l.SetPorts(ctx, vm, listed, c); err != nil {
		t.Fatal(err)
	}
	if running {
		t.Error("Free asked while the old forwarder ran")
	}
	got := (*starts)[len(*starts)-1]
	if len(*starts) != 2 || !slices.Equal(got[len(got)-4:], []string{"-reserve", "60022,30001", "-only", "5173"}) {
		t.Errorf("forwarders %q", *starts)
	}
	if !l.Forwards.Running(vm) {
		t.Error("no forwarder")
	}
	if p, err := l.Forwards.read(vm); err != nil || !slices.Equal(p.Ranges, listed.Ranges) {
		t.Errorf("record %+v, %v", p, err)
	}
	if err := l.SetPorts(ctx, vm, backend.Ports{}, c); err != nil {
		t.Fatal(err)
	}
	if l.Forwards.Running(vm) || len(*starts) != 2 {
		t.Errorf("none: forwarder running %v, started %q", l.Forwards.Running(vm), *starts)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if err := l.SetPorts(ctx, vm, backend.Ports{All: true}, c); err != nil {
		t.Fatal(err)
	}
	if len(*starts) != 2 || len(warned.Names()) != 0 {
		t.Errorf("stopped: started %q, warned %q", *starts, warned.Names())
	}
}

// The forwarder needs the hostagent's pid file.
func TestForwardMain(t *testing.T) {
	err := ForwardMain(t.Context(), []string{"-vm", "v", "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-port", "1"}, nil,
		func(string) (map[int]bool, error) { return nil, nil }, &strings.Builder{})
	if err == nil {
		t.Error("no lifeline: ran")
	}
}
