package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/msg"
)

// A start with ssh probes for sshd, has services.sh write its config with
// the VM's own key and the names of its sessions' variables (never their
// values), and starts no sshd daemon; the host key it made is read and
// pinned. SSH then answers with devvm _docker-connect as ProxyCommand,
// nothing published; devvm's shell is an ssh session.
func TestStartSSH(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if _, err := s.SSH(t.Context(), vm); msg.IDOf(err) != "vm_not_running" {
		t.Errorf("stopped: %v", err)
	}
	c := backend.Confinement{SSH: true, Env: map[string]string{"NO_PROXY": "localhost"}}
	if err := s.Start(t.Context(), vm, backend.Confinement{Env: c.Env}); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.stubs.Reset(t)
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	pub, err := vmssh.EnsureKey(s.sshKey(vm))
	if err != nil {
		t.Fatal(err)
	}
	// Its sessions get the environment from /etc/environment, where the
	// start put it.
	want := []string{
		"docker container start " + vm,
		"docker exec -i -u 0 " + vm + " sh -c " + envScript,
		"docker exec -u 0 " + vm + " sh -c " + sshdProbe,
		"docker exec -i -u 0 -e DEVVM_DOCKERD=off -e DEVVM_LOOP=off -e DEVVM_SSHD=inetd " + vm + " sh -s -- ssh u " + pub,
		"docker exec -u 0 " + vm + " cat /etc/ssh/ssh_host_ed25519_key.pub",
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if b, _ := os.ReadFile(filepath.Join(s.stub, "environment")); string(b) != "# devvm-proxy begin\nNO_PROXY=localhost\n# devvm-proxy end\n" {
		t.Errorf("/etc/environment's block: %q", b)
	}
	if b, err := os.ReadFile(s.knownHosts(vm)); err != nil || string(b) != vm+" ssh-ed25519 AAAAstub\n" {
		t.Errorf("known_hosts: %q, %v", b, err)
	}
	tgt, err := s.SSH(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	proxy := "'/devvm' '_docker-connect' '-docker' '/bin/docker' '-data' '" + s.Data + "' '" + vm + "'"
	wantOpts := []backend.SSHOption{
		{Key: "IdentityFile", Value: `"` + s.sshKey(vm) + `"`},
		{Key: "IdentitiesOnly", Value: "yes"},
		{Key: "PreferredAuthentications", Value: "publickey"},
	}
	if tgt.Host != vm || tgt.Port != 0 || tgt.User != "u" || tgt.ProxyCommand != proxy || !slices.Equal(tgt.Options, wantOpts) {
		t.Errorf("SSH = %+v", tgt)
	}
	if fi, err := os.Stat(s.sshKey(vm)); err != nil || fi.Mode().Perm() != 0o600 || filepath.Dir(s.sshKey(vm)) != filepath.Dir(s.Dirs.DockerVM(vm)) {
		t.Errorf("key: %v, %v", fi, err)
	}
	if strings.Contains(s.field(t, "containers/"+vm+"/mounts"), ".sock") {
		t.Error("a socket is mounted")
	}

	// devvm's shell: an ssh session, in the folder, forwarding no agent.
	sh, err := s.Shell(t.Context(), vm, "/p/it's")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sh.Args, " ")
	if sh.Path != "/bin/ssh" || sh.Args[0] != "ssh" || !strings.Contains(got, "-o ProxyCommand="+proxy+" -l u "+vm+" cd -- '/p/it'\\''s' && exec bash -l") ||
		!slices.Contains(sh.Args, "ForwardAgent=no") || !slices.Contains(sh.Args, "UserKnownHostsFile="+s.knownHosts(vm)) {
		t.Errorf("Shell = %q", sh.Args)
	}

	// Started again outside devvm (docker start): no ssh to reach, and a
	// shell through docker exec.
	if err := s.stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := s.Runner.Run(t.Context(), msg.M{}, s.docker(t.Context(), "container", "start", vm)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SSH(t.Context(), vm); msg.IDOf(err) != "vm_no_ssh" {
		t.Errorf("started outside devvm: %v", err)
	}
	if sh, err := s.Shell(t.Context(), vm, "/p"); err != nil || sh.Args[0] != "docker" {
		t.Errorf("Shell started outside devvm: %q, %v", sh.Args, err)
	}

	// Deleted: its key and pinned host key go.
	if err := s.Delete(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{s.sshKey(vm), s.knownHosts(vm)} {
		if _, err := os.Stat(f); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left: %v", f, err)
		}
	}
}

// An image without sshd: the VM starts and runs without ssh, warned.
func TestStartNoSSHD(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	s.stubs.Touch(t, "no-sshd")
	var warned msg.Recorder
	s.Warn = warned.Add
	s.stubs.Reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{SSH: true}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(warned.Names(), []string{"vm_no_sshd vm=" + vm}) || s.field(t, "containers/"+vm+"/status") != "running" {
		t.Errorf("warned %q", warned.Names())
	}
	if got := s.calls(t); got[len(got)-1] != "docker exec -i -u 0 -e DEVVM_DOCKERD=off -e DEVVM_LOOP=off -e DEVVM_SSHD=inetd "+vm+" sh -s --" {
		t.Errorf("calls: %q", got)
	}
	if _, err := s.SSH(t.Context(), vm); msg.IDOf(err) != "vm_no_sshd" {
		t.Errorf("SSH: %v", err)
	}
}

// A start's sessions forward an agent only when the VM forwards one and
// the start gives it one: the one devvm's SSH_AUTH_SOCK names, which devvm
// sets from the VM's choice. Forwarding changes only on a stopped VM.
func TestAgent(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web", AgentForwarding: true})
	if in, err := s.Inspect(t.Context(), vm); err != nil || !in.AgentForwarding || in.AgentFixed {
		t.Errorf("Inspect = %+v, %v", in, err)
	}
	for _, tc := range []struct {
		on    bool
		c     backend.Confinement
		agent string
		args  []string
	}{
		{true, backend.Confinement{SSH: true}, vmssh.AgentUser, []string{"ForwardAgent=yes", "IdentityAgent=SSH_AUTH_SOCK"}},
		// devvm's own: its socket is what devvm's SSH_AUTH_SOCK names then.
		{true, backend.Confinement{SSH: true, AgentSocket: "/d/agent.sock"}, vmssh.AgentUser, []string{"ForwardAgent=yes", "IdentityAgent=SSH_AUTH_SOCK"}},
		{true, backend.Confinement{SSH: true, NoAgent: true}, "", []string{"ForwardAgent=no", "IdentityAgent=none"}},
		{false, backend.Confinement{SSH: true}, "", []string{"ForwardAgent=no"}},
	} {
		if err := s.Stop(t.Context(), vm); err != nil {
			t.Fatal(err)
		}
		if err := s.SetAgentForwarding(t.Context(), vm, tc.on); err != nil {
			t.Fatal(err)
		}
		if err := s.Start(t.Context(), vm, tc.c); err != nil {
			t.Fatal(err)
		}
		r, _ := s.readRecord(vm)
		sh, err := s.Shell(t.Context(), vm, "/p/web")
		if err != nil || r.Agent != tc.agent {
			t.Errorf("%+v: agent %q, %v", tc.c, r.Agent, err)
		}
		for _, a := range tc.args {
			if !slices.Contains(sh.Args, a) {
				t.Errorf("%+v: shell lacks %s: %q", tc.c, a, sh.Args)
			}
		}
	}
	if err := s.SetAgentForwarding(t.Context(), vm, true); msg.IDOf(err) != "vm_not_stopped" {
		t.Errorf("running: %v", err)
	}
}

// Ports: none, listed or every one, recorded; a start forwards them
// through its forwarder, once its sshd's config is written: every port,
// or the listed ones Free leaves (those a host program has left out),
// through devvm _docker-connect, for this start of its container, and a
// running VM's change starts its forwarder again. Nothing is published.
func TestPorts(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web", Ports: backend.Ports{All: true}})
	if !strings.Contains(s.field(t, "containers/"+vm+"/mounts"), "dst=") || strings.Contains(strings.Join(s.calls(t), " "), " -p ") {
		t.Errorf("published: %q", s.calls(t))
	}
	short, err := os.MkdirTemp("/tmp", "dv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	s.Forwards.Run = short
	var args []string
	s.Forwards.Detach = func(c daemon.Cmd) (int, error) {
		args = append([]string{c.Path}, c.Args...)
		return os.Getpid(), daemon.WriteStatus(c.Status, os.Getpid(), "")
	}
	var warned msg.Recorder
	s.Warn = warned.Add
	start := func(c backend.Confinement) {
		t.Helper()
		args = nil
		if err := s.Stop(t.Context(), vm); err != nil {
			t.Fatal(err)
		}
		if err := s.Start(t.Context(), vm, c); err != nil {
			t.Fatal(err)
		}
	}
	start(backend.Confinement{SSH: true})
	ct, err := s.vm(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	proxy := "'/devvm' '_docker-connect' '-docker' '/bin/docker' '-data' '" + s.Data + "' '" + vm + "'"
	want := []string{
		"/devvm", "_docker-ports", "-vm", vm, "-docker", "/bin/docker", "-container", ct.ID, "-started", ct.State.StartedAt, "-proxy", proxy,
		"-user", "u", "-key", s.sshKey(vm), "-known-hosts", s.Forwards.KnownHosts(vm), "-control", s.Forwards.Control(vm), "-reserve", "",
	}
	if !slices.Equal(args, want) || ct.ID == "" || ct.State.StartedAt == "" || len(warned.Names()) != 0 {
		t.Errorf("every port:\n%q\nwant\n%q; warned %q", args, want, warned.Names())
	}
	if b, err := os.ReadFile(s.Forwards.KnownHosts(vm)); err != nil || string(b) != vm+" ssh-ed25519 AAAAstub\n" {
		t.Errorf("forwarder's known_hosts: %q, %v", b, err)
	}

	// Listed: those the start names.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	listed := backend.Ports{Ranges: []backend.PortRange{{From: 3000, To: 3000}, {From: 8000, To: 8002}}}
	args = nil
	if err := s.SetPorts(t.Context(), vm, listed, backend.Confinement{}); err != nil || args != nil {
		t.Fatalf("stopped: %q, %v", args, err)
	}
	if r, _ := s.readRecord(vm); r.PortsAll || !slices.Equal(r.Ports, listed.Ranges) {
		t.Errorf("record: %+v", r)
	}
	free := func(left []backend.PortRange) func(context.Context, []backend.PortRange) []backend.PortRange {
		return func(context.Context, []backend.PortRange) []backend.PortRange { return left }
	}
	start(backend.Confinement{SSH: true, Free: free(listed.Ranges[1:])})
	if i := slices.Index(args, "-only"); i < 0 || args[i+1] != "8000-8002" {
		t.Errorf("listed: %q", args)
	}
	// All of them taken: no forwarder.
	start(backend.Confinement{SSH: true, Free: free(nil)})
	if args != nil {
		t.Errorf("none left: %q", args)
	}
	// Without ssh: warned, no forwarder.
	start(backend.Confinement{})
	if args != nil || !slices.Contains(warned.Names(), "ports_no_ssh vm="+vm) {
		t.Errorf("without ssh: %q, warned %q", args, warned.Names())
	}
	// One that never says it forwards: warned, the VM runs on.
	warned.Reset()
	detach := s.Forwards.Detach
	s.Forwards.Detach = func(daemon.Cmd) (int, error) { return os.Getpid(), nil }
	start(backend.Confinement{SSH: true})
	if !slices.Equal(warned.Names(), []string{"ports_forward_failed log=" + s.Forwards.Log(vm) + " vm=" + vm}) {
		t.Errorf("warned %q", warned.Names())
	}
	s.Forwards.Detach = detach
	// None: nothing.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPorts(t.Context(), vm, backend.Ports{}, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	warned.Reset()
	args = nil
	if err := s.Start(t.Context(), vm, backend.Confinement{SSH: true}); err != nil || args != nil || len(warned.Names()) != 0 {
		t.Errorf("none: %q, %q, %v", args, warned.Names(), err)
	}
	// Running: its forwarder starts again at once with the new ports and
	// the host key its start pinned, for the same start of its container.
	was, err := s.vm(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPorts(t.Context(), vm, listed, backend.Confinement{Free: free(listed.Ranges[:1])}); err != nil {
		t.Fatal(err)
	}
	now, err := s.vm(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.Index(args, "-only"); i < 0 || args[i+1] != "3000" || !slices.Contains(args, now.State.StartedAt) || len(warned.Names()) != 0 {
		t.Errorf("running: %q, warned %q", args, warned.Names())
	}
	if b, err := os.ReadFile(s.Forwards.KnownHosts(vm)); err != nil || string(b) != vm+" ssh-ed25519 AAAAstub\n" {
		t.Errorf("running: forwarder's known_hosts: %q, %v", b, err)
	}
	if now.status() != backend.StatusRunning || now.State.StartedAt != was.State.StartedAt {
		t.Errorf("running: %+v", now.State)
	}
	args = nil
	if err := s.SetPorts(t.Context(), vm, backend.Ports{}, backend.Confinement{}); err != nil || args != nil {
		t.Errorf("running, none: %q, %v", args, err)
	}
}

// The forwarder of a Docker VM runs while Docker runs its container's
// start, and needs how to reach it.
func TestForwardMain(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	ct, err := s.vm(t.Context(), vm)
	if err != nil {
		t.Fatal(err)
	}
	if !containerRuns(t.Context(), "docker", ct.ID, ct.State.StartedAt) || containerRuns(t.Context(), "docker", ct.ID, "then") {
		t.Error("containerRuns: not this start alone")
	}
	reserved := func(string) (map[int]bool, error) { return nil, nil }
	base := []string{"-vm", vm, "-user", "u", "-key", "/k", "-known-hosts", "/kh", "-control", "/c", "-proxy", "'x'"}
	if err := ForwardMain(t.Context(), base, nil, reserved, &strings.Builder{}); err == nil {
		t.Error("no container: ran")
	}
	if err := ForwardMain(t.Context(), append(base, "-docker", "docker", "-container", ct.ID, "-started", "then"), nil, reserved, &strings.Builder{}); err == nil {
		t.Error("another start: ran")
	}
}

// ssh's ProxyCommand: devvm's words quoted, Docker's own variables as
// devvm has them; a word it can't quote is refused.
func TestProxyCommand(t *testing.T) {
	s := newSandbox(t)
	s.Getenv = func(k string) string {
		return map[string]string{"DOCKER_HOST": "ssh://me@box", "DOCKER_CERT_PATH": "/c"}[k]
	}
	got, err := s.proxyCommand(vm)
	if want := "'/devvm' '_docker-connect' '-docker' '/bin/docker' '-data' '" + s.Data + "' '-env' 'DOCKER_HOST=ssh://me@box' '" + vm + "'"; err != nil || got != want {
		t.Errorf("proxyCommand = %q, %v\nwant %q", got, err, want)
	}
	for _, bad := range []string{"/a'b", `/a\b`, "/a%h", "/a\nb", ""} {
		if _, err := shellWords([]string{bad}); msg.IDOf(err) != "docker_ssh_path" {
			t.Errorf("shellWords(%q): %v", bad, err)
		}
	}
	s.Self = "/odd'path/devvm"
	if _, err := s.proxyCommand(vm); msg.IDOf(err) != "docker_ssh_path" {
		t.Errorf("an odd path: %v", err)
	}
}

// devvm _docker-connect checks the container is this devvm's, a VM's and
// running, then runs docker exec on its ID: sshd in inetd mode, as root,
// on devvm's config; its arguments are checked first.
func TestConnect(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	var ran []string
	run := func(path string, argv, env []string) error {
		ran = append([]string{path}, argv...)
		if !slices.Contains(env, "DOCKER_CONTEXT=other") {
			return errors.New("no DOCKER_CONTEXT")
		}
		return nil
	}
	args := []string{"-docker", "docker", "-data", s.Data, "-env", "DOCKER_CONTEXT=other", vm}
	if err := Connect(t.Context(), args, run); msg.IDOf(err) != "vm_not_running" || ran != nil {
		t.Errorf("stopped: %v, ran %q", err, ran)
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	ct, _ := s.vm(t.Context(), vm)
	if err := Connect(t.Context(), args, run); err != nil {
		t.Fatal(err)
	}
	want := []string{"docker", "docker", "exec", "-i", "-u", "0", ct.ID, "/usr/sbin/sshd", "-i", "-f", "/etc/devvm/ssh/sshd_config", "-E", "/var/log/devvm-sshd.log"}
	if !slices.Equal(ran, want) {
		t.Errorf("ran %q\nwant %q", ran, want)
	}
	for name, a := range map[string][]string{
		"another devvm's": {"-docker", "docker", "-data", "/elsewhere", vm},
		"no such":         {"-docker", "docker", "-data", s.Data, "devvm-none-0123456789abcd"},
		"no VM":           {"-docker", "docker", "-data", s.Data},
		"other env":       {"-docker", "docker", "-data", s.Data, "-env", "PATH=/x", vm},
		"no data":         {"-docker", "docker", vm},
	} {
		ran = nil
		if err := Connect(t.Context(), a, run); err == nil || ran != nil {
			t.Errorf("%s: %v, ran %q", name, err, ran)
		}
	}
	s.stubs.Touch(t, "docker-foreign-"+vm)
	ran = nil
	if err := Connect(t.Context(), args, run); msg.IDOf(err) != "docker_container_foreign" || ran != nil {
		t.Errorf("someone else's: %v, ran %q", err, ran)
	}
}

// Stopping a VM stops its forwarder.
func TestStopStopsForwarder(t *testing.T) {
	s := newSandbox(t)
	s.createVM(t, backend.VMSpec{Project: "/p/web"})
	var asked []int
	s.Forwards.Args = func(pid int) ([]string, error) { asked = append(asked, pid); return nil, errors.New("gone") }
	if err := os.WriteFile(filepath.Join(s.Forwards.Dir, vm+".ports.pid"), []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if len(asked) == 0 {
		t.Error("its forwarder wasn't looked for")
	}
}
