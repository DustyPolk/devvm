package vmssh

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/state"
)

// testForwards is Forwards whose forwarders are processes of the test's
// (sleep), which say they forward at once: it returns each start's
// arguments and what it warned.
func testForwards(t *testing.T) (*Forwards, *[][]string, *msg.Recorder) {
	t.Helper()
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	var mu sync.Mutex
	var starts [][]string
	procs := map[int][]string{}
	warned := &msg.Recorder{}
	f := &Forwards{
		Dirs: paths.Dirs{Data: short}, Self: "/devvm", Tick: time.Millisecond,
		Detach: func(c daemon.Cmd) (int, error) {
			cmd := exec.CommandContext(t.Context(), "sleep", "30")
			if err := cmd.Start(); err != nil {
				return 0, err
			}
			done := make(chan struct{})
			go func() { _ = cmd.Wait(); close(done) }()
			t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
			mu.Lock()
			defer mu.Unlock()
			starts = append(starts, append([]string{c.Path}, c.Args...))
			procs[cmd.Process.Pid] = append([]string{c.Path}, c.Args...)
			return cmd.Process.Pid, daemon.WriteStatus(c.Status, cmd.Process.Pid, "")
		},
		Args: func(pid int) ([]string, error) {
			mu.Lock()
			defer mu.Unlock()
			if a, ok := procs[pid]; ok {
				return a, nil
			}
			return nil, errors.New("not ours")
		},
		InUse: func(context.Context, int) bool { return false },
		Warn:  warned.Add,
	}
	return f, &starts, warned
}

// The forwarder's start reads which ports the VM forwards from its state
// and how to reach it from its backend: every port, or its listed ones
// less those a host program has (warned), asked once its old forwarder
// has stopped; none, or all its listed ones taken, runs none. It never
// takes the VM's ssh or proxy port, and checks the host keys devvm pins
// for the VM's start.
func TestForwardsStart(t *testing.T) {
	f, starts, warned := testForwards(t)
	const vm = "devvm-p-0123456789abcd"
	key := "ssh-ed25519 " + keyED
	b, _ := hostKeysFake(t, vm, &key)
	// The forwarder's target is the VM's ssh, Options and ConfigFile
	// left out.
	b.Target = backend.SSHTarget{
		Host: Loopback, Port: 60022, User: "u", Key: "/k",
		Options: []backend.SSHOption{{Key: "IdentitiesOnly", Value: "yes"}}, ConfigFile: "/lima/ssh.config",
	}
	b.LifelineFile = "/lima/ha.pid"
	st, err := state.New(f.Dirs, vm)
	if err != nil {
		t.Fatal(err)
	}
	forward := func(list string) {
		t.Helper()
		if err := ports.Set(st, ports.Ports{Forward: list, Host: ports.Host{SSH: 60022, Proxy: 30001}}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := t.Context()
	forward("")
	f.Start(ctx, b, st)
	want := []string{
		"/devvm", "_ports", "-vm", vm, "-lifeline", "/lima/ha.pid", "-port", "60022", "-user", "u", "-key", "/k",
		"-known-hosts", f.Dirs.VM(vm).KnownHosts(), "-control", f.Control(vm), "-reserve", "60022,30001",
	}
	if len(*starts) != 1 || !slices.Equal((*starts)[0], want) || len(warned.Names()) != 0 {
		t.Fatalf("every port:\n%q\nwant\n%q\nwarned %q", *starts, want, warned.Names())
	}
	if got := readFile(t, f.Dirs.VM(vm).KnownHosts()); got != startLine+"start-0\n"+vm+" ssh-ed25519 "+keyED+"\n" {
		t.Errorf("known_hosts: %q", got)
	}
	if !f.Running(vm) {
		t.Error("not running")
	}

	// Listed: the free ones, asked with the old one stopped.
	asked := false
	f.InUse = func(_ context.Context, port int) bool {
		if f.Running(vm) {
			t.Error("asked while the old forwarder ran")
		}
		asked = true
		return port == 5173
	}
	forward("5173,8000-8010")
	f.Start(ctx, b, st)
	got := (*starts)[len(*starts)-1]
	if len(*starts) != 2 || !asked || !slices.Equal(got[len(got)-2:], []string{"-only", "8000-8010"}) {
		t.Errorf("listed: %q", *starts)
	}
	if w := warned.Names(); !slices.Equal(w, []string{"ports_taken count=1 ports=5173 vm=" + vm}) {
		t.Errorf("warned %q", w)
	}
	warned.Reset()

	// All taken, or none: no forwarder, the old one stopped.
	for _, list := range []string{"5173", "none"} {
		forward(list)
		f.Start(ctx, b, st)
		if len(*starts) != 2 || f.Running(vm) {
			t.Errorf("%s: started %q, running %v", list, *starts, f.Running(vm))
		}
	}
	warned.Reset()

	// What it can't start with is warned about, naming its log: a state it
	// can't read, an ssh target FromTarget refuses (another address, a
	// name, no user, no key, a relative key), a host key it can't read, a
	// VM its backend can't reach. None of them starts a forwarder.
	failed := []string{"ports_forward_failed log=" + f.Log(vm) + " vm=" + vm}
	forward("x")
	f.Start(ctx, b, st)
	if w := warned.Names(); !slices.Equal(w, failed) || len(*starts) != 2 {
		t.Errorf("a state it can't read: started %q, warned %q", *starts, w)
	}
	warned.Reset()
	forward("")
	good := b.Target
	for _, bad := range []backend.SSHTarget{
		{Host: "10.0.0.2", Port: 22, User: "u", Key: "/k"},
		{Host: "localhost", Port: 60022, User: "u", Key: "/k"},
		{Host: "0.0.0.0", Port: 60022, User: "u", Key: "/k"},
		{Host: Loopback, Port: 60022, Key: "/k"},
		{Host: Loopback, Port: 60022, User: "u"},
		{Host: Loopback, Port: 60022, User: "u", Key: "k"},
		{Host: Loopback, User: "u", Key: "/k"},
	} {
		b.Target = bad
		f.Start(ctx, b, st)
		if w := warned.Names(); !slices.Equal(w, failed) || len(*starts) != 2 || f.Running(vm) {
			t.Errorf("%+v: started %q, warned %q", bad, *starts, w)
		}
		warned.Reset()
	}
	b.Target = good
	key = "no keys"
	if err := ForgetHostKeys(f.Dirs, vm); err != nil {
		t.Fatal(err)
	}
	f.Start(ctx, b, st)
	key = "ssh-ed25519 " + keyED
	b.Instances[vm] = backend.Instance{Name: vm, Status: backend.StatusStopped}
	f.Start(ctx, b, st)
	if w := warned.Names(); len(*starts) != 2 || !slices.Equal(w, slices.Concat(failed, failed)) {
		t.Errorf("started %q, warned %q", *starts, w)
	}
}

// splitPorts splits listed ports into those free and those in use, each
// as ranges.
func TestSplitPorts(t *testing.T) {
	r := func(a, b int) backend.PortRange { return backend.PortRange{From: a, To: b} }
	inUse := func(p int) bool { return p == 5173 || p == 5175 || p == 8080 || p == 8081 }
	free, taken := splitPorts([]backend.PortRange{r(5173, 5173), r(5174, 5176), r(5177, 5177), r(8079, 8082)}, inUse)
	if want := []backend.PortRange{r(5174, 5174), r(5176, 5176), r(5177, 5177), r(8079, 8079), r(8082, 8082)}; !slices.Equal(free, want) {
		t.Errorf("free %v, want %v", free, want)
	}
	if want := []backend.PortRange{r(5173, 5173), r(5175, 5175), r(8080, 8081)}; !slices.Equal(taken, want) {
		t.Errorf("taken %v, want %v", taken, want)
	}
}
