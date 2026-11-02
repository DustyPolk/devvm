package smolvm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// A forwarder needs the machine's lifeline.
func TestForwardMainLifeline(t *testing.T) {
	if err := ForwardMain(t.Context(), []string{"-vm", "v", "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-port", "1"}, nil,
		func(string) (map[int]bool, error) { return nil, nil }, &strings.Builder{}); err == nil {
		t.Error("no lifeline: ran")
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
	var args []string
	s.fakeForwarder(t, vm, &args)
	running := false
	s.Forwards.Args = func(int) ([]string, error) {
		if !running {
			return nil, errors.New("gone")
		}
		return []string{"/devvm", forwarderCmd, "-vm", vm}, nil
	}
	var warned msg.Recorder
	s.Warn = warned.Add
	c := backend.Confinement{SSH: true, Net: backend.NetDirect, ProxyPort: 30001, Decider: "127.0.0.1:30002"}
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if len(warned.Names()) != 0 {
		t.Errorf("warned %q", warned.Names())
	}
	r, _ := s.readRecord(vm)
	want := []string{
		"/devvm", "_smolvm-ports", "-vm", vm, "-lifeline", filepath.Join(s.stub, "smolvm-data", vm, "agent.pid"),
		"-port", "61022", "-user", "u", "-key", s.sshKey(vm), "-known-hosts", s.Forwards.KnownHosts(vm),
		"-control", s.Forwards.Control(vm), "-reserve", "61022,30001,30002",
	}
	if !r.PortsAll || !slices.Equal(args, want) {
		t.Errorf("forwarder:\n%q\nwant\n%q", args, want)
	}
	if !strings.HasPrefix(filepath.Base(s.Forwards.Control(vm)), "smolvm-") || filepath.Dir(s.Forwards.KnownHosts(vm)) != filepath.Dir(s.Dirs.SmolvmVM(vm)) {
		t.Errorf("its files: %s, %s", s.Forwards.Control(vm), s.Forwards.KnownHosts(vm))
	}
	if b, err := os.ReadFile(s.Forwards.KnownHosts(vm)); err != nil || string(b) != vm+" ssh-ed25519 AAAAstub\n" {
		t.Errorf("known_hosts: %q, %v", b, err)
	}
	if !slices.Contains(s.calls(t), "smolvm machine exec --name "+vm+" -- cat /etc/ssh/ssh_host_ed25519_key.pub") {
		t.Errorf("host key not read through smolvm: %q", s.calls(t))
	}
	running = true
	if !s.Forwards.Running(vm) {
		t.Error("forwarder not found")
	}
	running = false

	// Without sshd: warned, no forwarder.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.stubs.Touch(t, "no-sshd")
	args = nil
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if args != nil || !slices.Contains(warned.Names(), "ports_no_ssh vm="+vm) {
		t.Errorf("no sshd: forwarder %q, warned %q", args, warned.Names())
	}
	if err := os.Remove(filepath.Join(s.stub, "no-sshd")); err != nil {
		t.Fatal(err)
	}
	// A forwarder that never says it forwards: warned, the VM runs on.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	warned.Reset()
	s.Forwards.Detach = func(string, []string, *os.File) (int, error) { return os.Getpid(), nil }
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(warned.Names(), []string{"ports_forward_failed log=" + s.Forwards.Log(vm) + " vm=" + vm}) {
		t.Errorf("warned %q", warned.Names())
	}

	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".ports.pid", ".ports.known_hosts", ".ports.log"} {
		if _, err := os.Stat(filepath.Join(s.Forwards.Dir, vm+ext)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", ext, err)
		}
	}
}

// fakeForwarder makes vm's forwarder one that says it forwards at once,
// its arguments kept in args, with a short folder for its control socket:
// a test's temporary one can be too long for one.
func (s sandbox) fakeForwarder(t *testing.T, vm string, args *[]string) {
	t.Helper()
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	s.Forwards.Run = short
	s.Forwards.Detach = func(path string, a []string, out *os.File) (int, error) {
		*args = append([]string{path}, a...)
		_, err := fmt.Fprintf(out, "devvm ports: forwarding %s's ports\n", vm)
		return os.Getpid(), err
	}
}

// A VM's listed ports are recorded, none of smolvm's, however many: its
// forwarder forwards them alone (-only), those Confinement.Free leaves (a
// host program's left out); with none, no forwarder runs. A running VM's
// change starts its forwarder again with the new ports.
func TestStartListedForwarder(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-l-0123456789abcd"
	s.image(t, "0123456789ab")
	listed := []backend.PortRange{{From: 22, To: 22}, {From: 3000, To: 3000}, {From: 8000, To: 8100}}
	if err := s.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{Name: vm, Ports: backend.Ports{Ranges: listed}}); err != nil {
		t.Fatal(err)
	}
	var args []string
	var warned msg.Recorder
	s.fakeForwarder(t, vm, &args)
	s.Warn = warned.Add
	c := backend.Confinement{SSH: true, Net: backend.NetDirect}
	for _, tc := range []struct {
		free []backend.PortRange // nil for no Free
		only string              // "" for no forwarder
	}{
		{nil, "22,3000,8000-8100"},
		{listed[2:], "8000-8100"},
		{[]backend.PortRange{}, ""},
	} {
		args = nil
		warned.Reset()
		c.Free = nil
		if tc.free != nil {
			c.Free = func(_ context.Context, rs []backend.PortRange) []backend.PortRange {
				if !slices.Equal(rs, listed) {
					t.Errorf("Free asked of %v", rs)
				}
				return tc.free
			}
		}
		if err := s.Start(t.Context(), vm, c); err != nil {
			t.Fatal(err)
		}
		i := slices.Index(args, "-only")
		if tc.only == "" && args != nil || tc.only != "" && (i < 0 || i+1 >= len(args) || args[i+1] != tc.only) || len(warned.Names()) != 0 {
			t.Errorf("only %q: forwarder %q, warned %q", tc.only, args, warned.Names())
		}
		if err := s.Stop(t.Context(), vm); err != nil {
			t.Fatal(err)
		}
	}
	for _, call := range s.calls(t) {
		if strings.HasPrefix(call, "smolvm machine") && strings.Contains(call, " -p ") && !strings.HasSuffix(call, ":22") {
			t.Errorf("smolvm forwards a listed port: %q", call)
		}
	}
	// Every port again: the record alone changes.
	s.stubs.Reset(t)
	args = nil
	if err := s.SetPorts(t.Context(), vm, backend.Ports{All: true}, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.readRecord(vm); !r.PortsAll || r.Listed != nil || len(s.calls(t)) != 0 || args != nil {
		t.Errorf("all: %+v, %q, forwarder %q", r, s.calls(t), args)
	}

	// Running: its forwarder starts again with the new ports at once, the
	// VM left running; none starts none.
	c.Free = nil
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(args, "-only") {
		t.Errorf("all: forwarder %q", args)
	}
	s.stubs.Reset(t)
	args = nil
	c = backend.Confinement{ProxyPort: 30001, Free: func(_ context.Context, rs []backend.PortRange) []backend.PortRange { return rs[1:] }}
	if err := s.SetPorts(t.Context(), vm, backend.Ports{Ranges: listed}, c); err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(args, "-only"); i < 0 || args[i+1] != "3000,8000-8100" || !slices.Contains(args, "61022,30001") {
		t.Errorf("running: forwarder %q", args)
	}
	for _, call := range s.calls(t) {
		if strings.Contains(call, "machine stop") || strings.Contains(call, "machine start") {
			t.Errorf("running: %q", call)
		}
	}
	args = nil
	if err := s.SetPorts(t.Context(), vm, backend.Ports{}, c); err != nil || args != nil {
		t.Errorf("running, none: forwarder %q, %v", args, err)
	}
	// A forward given a record read before another devvm's change: the
	// record's ports as they are now (none), so no forwarder.
	stale, err := s.readRecord(vm)
	if err != nil {
		t.Fatal(err)
	}
	stale.Listed = listed
	args = nil
	if s.forward(t.Context(), vm, stale, c); args != nil {
		t.Errorf("stale record: forwarder %q", args)
	}
	// Started outside devvm since (another lifeline): no sshd of devvm's,
	// so no forwarder, warned; the record changes all the same.
	other := []byte(strconv.Itoa(os.Getpid()) + "\n1")
	if err := os.WriteFile(filepath.Join(s.stub, "smolvm-data", vm, "agent.pid"), other, 0o600); err != nil {
		t.Fatal(err)
	}
	args = nil
	warned.Reset()
	if err := s.SetPorts(t.Context(), vm, backend.Ports{Ranges: listed}, c); err != nil || args != nil ||
		!slices.Equal(warned.Names(), []string{"ports_no_ssh vm=" + vm}) {
		t.Errorf("started elsewhere: forwarder %q, warned %q, %v", args, warned.Names(), err)
	}
	if r, _ := s.readRecord(vm); !slices.Equal(r.Listed, listed) {
		t.Errorf("started elsewhere: record %+v", r)
	}
}
