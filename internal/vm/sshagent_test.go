package vm

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/sshagent"
)

// --ssh-agent: the new VM forwards the user's agent, which is checked
// first, and devvm's connections to it carry it. Without, they carry none.
func TestUpSSHAgent(t *testing.T) {
	for _, on := range []bool{true, false} {
		e, f, _ := upEnv(t, allNative())
		var adds []string
		e.SSHAdd = func(_ context.Context, args ...string) int { adds = append(adds, strings.Join(args, " ")); return 0 }
		st := existingVM(t, e, Options{NoNetworkPolicy: true, SSHAgent: on})
		spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
		mode, sock := sshagent.Off, ""
		if on {
			mode, sock = sshagent.On, "/user/agent"
		}
		if spec.AgentForwarding != on || get(st, keyAgent) != mode {
			t.Errorf("--ssh-agent %v: forwarding %v, state %q", on, spec.AgentForwarding, get(st, keyAgent))
		}
		if got := os.Getenv("SSH_AUTH_SOCK"); got != sock {
			t.Errorf("--ssh-agent %v: SSH_AUTH_SOCK=%q, want %q", on, got, sock)
		}
		if want := map[bool][]string{true: {"-l"}, false: nil}[on]; !slices.Equal(adds, want) {
			t.Errorf("--ssh-agent %v: ssh-add calls %q", on, adds)
		}
		if c, _ := call(f, "Start").Args[2].(backend.Confinement); c.AgentSocket != "" {
			t.Errorf("--ssh-agent %v: started with agent %q", on, c.AgentSocket)
		}
	}
}

// A VM with network policy whose mode is devvm starts confined with
// devvm's agent too: the proxy's confinement keeps it.
func TestOpenAgentNetworkPolicy(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	e.Agent.Run = (&agentCalls{}).run
	st := existingVM(t, e, Options{NetworkPolicy: true, SSHAgent: true})
	vm := st.Name()
	if err := set(t, st, keyAgent, sshagent.Devvm); err != nil {
		t.Fatal(err)
	}
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	calls := f.Calls()[before:]
	start := slices.IndexFunc(calls, func(c backendtest.Call) bool { return c.Method == "Start" })
	if start < 0 {
		t.Fatalf("%s didn't start: %v", vm, calls)
	}
	if c, _ := calls[start].Args[2].(backend.Confinement); c.Net != backend.NetProxy || c.AgentSocket != e.Agent.Socket() || c.ProxyPassword == "" || c.NoAgent {
		t.Errorf("started with %+v, want the proxy and agent %q", c, e.Agent.Socket())
	}
	// Off: the proxy's confinement says no agent too.
	if err := set(t, st, keyAgent, sshagent.Off); err != nil {
		t.Fatal(err)
	}
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if c, _ := call(f, "Start").Args[2].(backend.Confinement); c.Net != backend.NetProxy || c.AgentSocket != "" || !c.NoAgent {
		t.Errorf("off: started with %+v", c)
	}
}

// A VM whose mode is devvm starts with devvm's agent, its keys loaded,
// and devvm's connections to it carry that agent; a stopped VM's backend
// forwarding is made to match its mode, a running one's waits for its
// next start. --ssh-agent on an existing VM is ignored.
func TestOpenAgent(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	agent := &agentCalls{}
	e.Agent.Run = agent.run
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	if err := set(t, st, keyAgent, sshagent.Devvm); err != nil {
		t.Fatal(err)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{SSHAgent: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	calls := f.Calls()[before:]
	start := slices.IndexFunc(calls, func(c backendtest.Call) bool { return c.Method == "Start" })
	if start < 0 {
		t.Fatalf("calls = %v", calls)
	}
	// Its start turns forwarding on.
	if w, _ := calls[start].Args[1].(backend.Want); !w.AgentForwarding || !f.Instances[vm].AgentForwarding {
		t.Errorf("started wanting %+v", w)
	}
	if c, _ := calls[start].Args[2].(backend.Confinement); c.AgentSocket != e.Agent.Socket() {
		t.Errorf("started with agent %q, want %q", c.AgentSocket, e.Agent.Socket())
	}
	if !slices.Contains(agent.calls, "ssh-add -l") {
		t.Errorf("devvm's agent wasn't checked: %q", agent.calls)
	}
	if got := os.Getenv("SSH_AUTH_SOCK"); got != e.Agent.Socket() {
		t.Errorf("SSH_AUTH_SOCK=%q", got)
	}
	// Running: its agent is checked again; a change waits.
	agent.calls = nil
	before = len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !slices.Contains(agent.calls, "ssh-add -l") || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("running: agent calls %q, calls %q", agent.calls, f.Methods()[before:])
	}
	if err := set(t, st, keyAgent, sshagent.Off); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if slices.Contains(f.Methods()[before:], "Start") || !f.Instances[vm].AgentForwarding {
		t.Errorf("running, turned off: calls %q", f.Methods()[before:])
	}
	// devvm's connections keep the agent it started with.
	if got := os.Getenv("SSH_AUTH_SOCK"); got != e.Agent.Socket() {
		t.Errorf("running, turned off: SSH_AUTH_SOCK=%q, want %q", got, e.Agent.Socket())
	}
	if err := (Launch{e}).Restart(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if f.Instances[vm].AgentForwarding {
		t.Error("restart didn't turn forwarding off")
	}
	if _, ok := os.LookupEnv("SSH_AUTH_SOCK"); ok {
		t.Error("restarted: SSH_AUTH_SOCK is set for a VM without forwarding")
	}
}

// A running VM runs with the agent choice its start was made with, as
// devvm recorded it; one started some other way, or stopped, with its
// choice.
func TestRunningAgent(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	st := existingVM(t, e, Options{NoNetworkPolicy: true, SSHAgent: true})
	vm := st.Name()
	if err := set(t, st, keyAgent, sshagent.Devvm); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		do   func()
		want string
	}{
		{"started by devvm", func() {}, sshagent.On},
		{"started outside devvm", func() { f.Restart(vm) }, sshagent.Devvm},
		{"stopped", func() { _ = f.Stop(t.Context(), vm) }, sshagent.Devvm},
	} {
		c.do()
		if got, err := runningAgent(t.Context(), f, st); err != nil || got != c.want {
			t.Errorf("%s: %q, %v; want %q", c.name, got, err, c.want)
		}
	}
}

// devvm ssh-agent: shown; changed in the state only, from the VM's next
// start. devvm needs its agent running; forwarding needs the backend's
// feature.
func TestSetSSHAgent(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	agent := &agentCalls{}
	e.Agent.Run = agent.run
	s := Settings{e}
	if _, err := s.SSHAgent(t.Context()); msg.NamesOf(err) != "no_vm_ssh_agent dir="+e.Dir {
		t.Errorf("no VM: %s", msg.NamesOf(err))
	}
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	shown := func(want string) {
		t.Helper()
		if mode, err := s.SSHAgent(t.Context()); err != nil || mode != want {
			t.Errorf("SSHAgent = %q, %v; want %s", mode, msg.NamesOf(err), want)
		}
	}
	shown("off")
	before := len(f.Calls())
	if err := s.SetSSHAgent(t.Context(), sshagent.Devvm); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	shown("devvm")
	if !slices.Contains(agent.calls, "ssh-add -l") {
		t.Errorf("devvm's agent wasn't started: %q", agent.calls)
	}
	if slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("calls = %q", f.Methods()[before:])
	}
	if err := s.SetSSHAgent(t.Context(), sshagent.Devvm); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := s.SetSSHAgent(t.Context(), sshagent.Off); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	shown("off")
	caps := allNative()
	caps[backend.FeatureAgentForwarding] = backend.None
	f.Capabilities = caps
	if err := s.SetSSHAgent(t.Context(), sshagent.On); !strings.HasPrefix(msg.NamesOf(err), "backend_lacks ") {
		t.Errorf("without the feature: %s", msg.NamesOf(err))
	}
	if sshagent.Mode(st) != sshagent.Off {
		t.Errorf("mode = %s", sshagent.Mode(st))
	}
}

// devvm ssh-agent on or devvm warns who can use the keys while you're
// connected.
func TestSetSSHAgentRisk(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	e.Agent.Run = (&agentCalls{}).run
	var warned msg.Recorder
	e.Warn = warned.Add
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	for _, mode := range []string{sshagent.On, sshagent.Devvm} {
		if err := (Settings{e}).SetSSHAgent(t.Context(), mode); err != nil {
			t.Fatal(msg.NamesOf(err))
		}
	}
	want := []string{"agent_on_risk vm=" + vm, "agent_devvm_risk vm=" + vm}
	if !slices.Equal(warned.Names(), want) {
		t.Errorf("warned %q, want %q", warned.Names(), want)
	}
}

// A backend that emulates turning forwarding on or off (agent-toggle):
// a new VM is made ready for it only while it's let through (warned when
// unreviewed), and its state says so; each start says whether the VM gets
// no agent; once it isn't let through, that VM no longer starts. A VM made
// without forwarding where it's fixed can't have it turned on.
func TestAgentToggleEmulated(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureAgentToggle] = backend.Emulated(false)
	e, f, _ := upEnv(t, caps)
	var warned msg.Recorder
	e.Registry.Warn = warned.Add
	e.Registry.Unreviewed = map[string]bool{"fake:agent-toggle": true}
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	vm := st.Name()
	if spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec); !spec.AgentToggle || spec.AgentForwarding {
		t.Errorf("spec = %+v", spec)
	}
	if !slices.Contains(warned.Names(), "backend_unreviewed_used backend=fake feature=agent-toggle vm="+vm) || get(st, KeyAgentToggle) != "on" {
		t.Errorf("warned %q, state %q", warned.Names(), get(st, KeyAgentToggle))
	}
	if c, _ := call(f, "Start").Args[2].(backend.Confinement); !c.NoAgent || c.AgentSocket != "" {
		t.Errorf("off: started with %+v", c)
	}
	// devvm's own, from its next start.
	if err := (Settings{e}).SetSSHAgent(t.Context(), sshagent.Devvm); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := (Launch{e}).Restart(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if c, _ := call(f, "Start").Args[2].(backend.Confinement); c.NoAgent || c.AgentSocket != e.Agent.Socket() || !f.Instances[vm].AgentForwarding {
		t.Errorf("devvm: started with %+v", c)
	}
	// No longer let through: it doesn't start.
	e.Registry.Unreviewed = nil
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); msg.NamesOf(err) != "backend_unreviewed backend=fake feature=agent-toggle vm="+vm {
		t.Errorf("refused: %s", msg.NamesOf(err))
	}
	// Made without forwarding, where that's fixed: on is refused.
	e.Registry.Unreviewed = map[string]bool{"fake:agent-toggle": true}
	in := f.Instances[vm]
	in.AgentForwarding, in.AgentFixed = false, true
	f.Instances[vm] = in
	if err := set(t, st, keyAgent, sshagent.Off); err != nil {
		t.Fatal(err)
	}
	if err := (Settings{e}).SetSSHAgent(t.Context(), sshagent.On); msg.NamesOf(err) != "agent_fixed backend=fake vm="+vm {
		t.Errorf("fixed, devvm ssh-agent on: %s", msg.NamesOf(err))
	}
	if err := set(t, st, keyAgent, sshagent.On); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{}); msg.NamesOf(err) != "agent_fixed backend=fake vm="+vm {
		t.Errorf("fixed, launched on: %s", msg.NamesOf(err))
	}
}

// Without agent-toggle let through, a new VM isn't made ready for it.
func TestAgentToggleRefused(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureAgentToggle] = backend.Emulated(false)
	e, f, _ := upEnv(t, caps)
	st := existingVM(t, e, Options{NoNetworkPolicy: true})
	if spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec); spec.AgentToggle || get(st, KeyAgentToggle) != "" {
		t.Errorf("spec = %+v, state %q", spec, get(st, KeyAgentToggle))
	}
	if err := (Settings{e}).SetSSHAgent(t.Context(), sshagent.On); msg.NamesOf(err) != "backend_unreviewed backend=fake feature=agent-toggle vm="+st.Name() {
		t.Errorf("devvm ssh-agent on: %s", msg.NamesOf(err))
	}
}

// checkAgent offers to load a default key only into an agent that runs
// with none; without a terminal it says how.
func TestCheckAgent(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rc    int    // ssh-add -l's
		key   string // in ~/.ssh
		os    string
		calls []string
	}{
		{name: "keys loaded", rc: 0, key: "id_ed25519", os: "darwin", calls: []string{"-l"}},
		{name: "no default key", rc: 1, os: "darwin", calls: []string{"-l"}},
		{name: "no agent", rc: 2, key: "id_rsa", os: "darwin", calls: []string{"-l"}},
		{name: "empty, macOS", rc: 1, key: "id_ecdsa", os: "darwin", calls: []string{"-l"}},
		{name: "empty, Linux", rc: 1, key: "id_ed25519", os: "linux", calls: []string{"-l"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.key != "" {
				if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".ssh", tc.key), []byte("k"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			e := &Env{Dirs: paths.Dirs{Home: home}, OS: tc.os, SSHAdd: func(_ context.Context, args ...string) int {
				calls = append(calls, strings.Join(args, " "))
				return tc.rc
			}}
			var said strings.Builder
			e.Warn = func(m msg.M) { said.WriteString("devvm: " + m.Names() + "\n") }
			e.Confirm = func(msg.M) bool { return false }
			e.Terminal = func() bool { return true }
			Launch{e}.checkAgent(t.Context())
			out := said.String()
			if !slices.Equal(calls, tc.calls) {
				t.Errorf("ssh-add calls = %q, want %q", calls, tc.calls)
			}
			want := map[string]string{
				"keys loaded":    "",
				"no default key": "",
				"no agent":       "devvm: no_ssh_agent\n",
				"empty, macOS": "devvm: ssh_agent_empty\ndevvm: load_key_with cmd=ssh-add --apple-use-keychain " +
					filepath.Join(home, ".ssh", "id_ecdsa") + "\n",
				"empty, Linux": "devvm: ssh_agent_empty\ndevvm: load_key_with cmd=ssh-add " + filepath.Join(home, ".ssh", "id_ed25519") + "\n",
			}[tc.name]
			if out != want {
				t.Errorf("stderr = %q, want %q", out, want)
			}
		})
	}
}
