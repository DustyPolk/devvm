package smolvm

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	// A short folder for its control socket: a test's temporary one can
	// be too long for one.
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	s.Forwards.Run = short
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
	s.touch(t, "no-sshd")
	args = nil
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if args != nil || !slices.Contains(warned, "ports_no_ssh vm="+vm) {
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
	if !slices.Equal(warned, []string{"ports_forward_failed log=" + s.Forwards.Log(vm) + " vm=" + vm}) {
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
		if _, err := os.Stat(filepath.Join(s.Forwards.Dir, vm+ext)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", ext, err)
		}
	}
}
