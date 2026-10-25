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
	"github.com/w3cj/devvm/internal/sshagent"
)

// --ssh-agent: the new VM forwards the user's agent, which is checked
// first, and devvm's connections to it carry it. Without, they carry none.
func TestUpSSHAgent(t *testing.T) {
	for _, on := range []bool{true, false} {
		e, f, _ := upEnv(t, allNative())
		var adds []string
		e.SSHAdd = func(_ context.Context, args ...string) int { adds = append(adds, strings.Join(args, " ")); return 0 }
		st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true, SSHAgent: on})
		spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
		mode, sock := sshagent.Off, ""
		if on {
			mode, sock = sshagent.On, "/user/agent"
		}
		if spec.AgentForwarding != on || st.Get(sshagent.Key) != mode {
			t.Errorf("--ssh-agent %v: forwarding %v, state %q", on, spec.AgentForwarding, st.Get(sshagent.Key))
		}
		if got := os.Getenv("SSH_AUTH_SOCK"); got != sock {
			t.Errorf("--ssh-agent %v: SSH_AUTH_SOCK=%q, want %q", on, got, sock)
		}
		if want := map[bool][]string{true: {"-l"}, false: nil}[on]; !slices.Equal(adds, want) {
			t.Errorf("--ssh-agent %v: ssh-add calls %q", on, adds)
		}
		if c, _ := call(f, "Start").Args[1].(backend.Confinement); c.AgentSocket != "" {
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
	st := existingVM(t, e, Options{Image: "base", NetworkPolicy: true, SSHAgent: true})
	vm := st.Name()
	if err := st.Set(sshagent.Key, sshagent.Devvm); err != nil {
		t.Fatal(err)
	}
	if err := (Lifecycle{e}).Stop(t.Context()); err != nil {
		t.Fatal(names(err))
	}
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	calls := f.Calls()[before:]
	start := slices.IndexFunc(calls, func(c backendtest.Call) bool { return c.Method == "Start" })
	if start < 0 {
		t.Fatalf("%s didn't start: %v", vm, calls)
	}
	if c, _ := calls[start].Args[1].(backend.Confinement); c.Net != backend.NetProxy || c.AgentSocket != e.Agent.Socket() || c.ProxyPassword == "" {
		t.Errorf("started with %+v, want the proxy and agent %q", c, e.Agent.Socket())
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
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	vm := st.Name()
	if err := st.Set(sshagent.Key, sshagent.Devvm); err != nil {
		t.Fatal(err)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{SSHAgent: true}); err != nil {
		t.Fatal(names(err))
	}
	calls := f.Calls()[before:]
	set := slices.IndexFunc(calls, func(c backendtest.Call) bool { return c.Method == "SetAgentForwarding" })
	start := slices.IndexFunc(calls, func(c backendtest.Call) bool { return c.Method == "Start" })
	if set < 0 || start < set || calls[set].Args[1] != true {
		t.Fatalf("calls = %v", calls)
	}
	if c, _ := calls[start].Args[1].(backend.Confinement); c.AgentSocket != e.Agent.Socket() {
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
		t.Fatal(names(err))
	}
	if !slices.Contains(agent.calls, "ssh-add -l") || slices.Contains(f.Methods()[before:], "Start") {
		t.Errorf("running: agent calls %q, calls %q", agent.calls, f.Methods()[before:])
	}
	if err := st.Set(sshagent.Key, sshagent.Off); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if slices.Contains(f.Methods()[before:], "SetAgentForwarding") || !f.Instances[vm].AgentForwarding {
		t.Errorf("running, turned off: calls %q", f.Methods()[before:])
	}
	if _, ok := os.LookupEnv("SSH_AUTH_SOCK"); ok {
		t.Error("SSH_AUTH_SOCK is set for a VM without forwarding")
	}
	if err := (Launch{e}).Restart(t.Context(), Options{}); err != nil {
		t.Fatal(names(err))
	}
	if f.Instances[vm].AgentForwarding {
		t.Error("restart didn't turn forwarding off")
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
	var b strings.Builder
	if err := s.SSHAgent(t.Context(), &b); names(err) != "no_vm_ssh_agent dir="+e.Dir {
		t.Errorf("no VM: %s", names(err))
	}
	st := existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	shown := func(want string) {
		t.Helper()
		b.Reset()
		if err := s.SSHAgent(t.Context(), &b); err != nil || b.String() != want+"\n" {
			t.Errorf("SSHAgent = %q, %v; want %s", b.String(), names(err), want)
		}
	}
	shown("off")
	before := len(f.Calls())
	if err := s.SetSSHAgent(t.Context(), sshagent.Devvm); err != nil {
		t.Fatal(names(err))
	}
	shown("devvm")
	if !slices.Contains(agent.calls, "ssh-add -l") {
		t.Errorf("devvm's agent wasn't started: %q", agent.calls)
	}
	if slices.Contains(f.Methods()[before:], "SetAgentForwarding") {
		t.Errorf("calls = %q", f.Methods()[before:])
	}
	if err := s.SetSSHAgent(t.Context(), sshagent.Devvm); err != nil {
		t.Fatal(names(err))
	}
	if err := s.SetSSHAgent(t.Context(), sshagent.Off); err != nil {
		t.Fatal(names(err))
	}
	shown("off")
	caps := allNative()
	caps[backend.FeatureAgentForwarding] = backend.None
	f.Capabilities = caps
	if err := s.SetSSHAgent(t.Context(), sshagent.On); !strings.HasPrefix(names(err), "backend_lacks ") {
		t.Errorf("without the feature: %s", names(err))
	}
	if sshagent.Mode(st) != sshagent.Off {
		t.Errorf("mode = %s", sshagent.Mode(st))
	}
}

// checkAgent offers to load a default key only into an agent that runs
// with none; without a terminal it says how.
func TestCheckAgent(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
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
			e := &Env{Home: home, OS: tc.os, SSHAdd: func(_ context.Context, args ...string) int {
				calls = append(calls, strings.Join(args, " "))
				return tc.rc
			}}
			var said strings.Builder
			e.Warn = func(m msg.M) { said.WriteString("devvm: " + m.String() + "\n") }
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
