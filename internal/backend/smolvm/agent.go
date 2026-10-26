package smolvm

import (
	"context"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// smolvm sets a machine's SSH agent forwarding only when it's made
// (--ssh-agent), and forwards the agent SSH_AUTH_SOCK names when it starts:
// its process connects to that path again for each of the VM's connections
// to the agent. So turning forwarding on or off later (agent-toggle) is
// emulated, reviewed: a machine made while devvm lets it through
// (VMSpec.AgentToggle) forwards from the start, and each start gives it
// the agent devvm's choice names: the user's (SSH_AUTH_SOCK), devvm's own
// (Confinement.AgentSocket), or none (noAgent). One started any other way
// is caught by its lifeline (AgentUnchosen), and devvm stops it.

// noAgent is the agent a machine that forwards one gets for none: a path
// under /dev/null, which is a device, not a folder, so no socket can ever
// be there, and each of the VM's connections to its agent fails. Nothing
// listens for it: no agent to add keys to, or to sign with.
const noAgent = "/dev/null/devvm-no-agent"

// SetAgentForwarding turns a stopped VM's forwarding on or off from its
// next start: only for one made forwarding, as smolvm can't add it to one
// made without (agent_fixed).
func (s *Smolvm) SetAgentForwarding(ctx context.Context, vm string, on bool) error {
	_, r, err := s.stopped(ctx, vm)
	if err != nil {
		return err
	}
	switch {
	case !r.Forwarding && !on, r.Forwarding && r.AgentOff != on:
		return nil
	case !r.Forwarding:
		return msg.AgentFixed(Name, vm)
	}
	r.AgentOff = !on
	return s.writeRecord(vm, r)
}

// AgentUnchosen reports whether a running machine made forwarding may
// forward an agent devvm didn't choose: one not running on the process
// Start last started (smolvm machine start from the user's shell forwards
// that shell's). A machine made without forwarding has no agent however
// it's started, and a stopped one none.
func (s *Smolvm) AgentUnchosen(ctx context.Context, vm string) (bool, error) {
	r, err := s.readRecord(vm)
	if err != nil || !r.Forwarding {
		return false, err
	}
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return false, notFound(vm, err)
	}
	if m.status() != backend.StatusRunning {
		return false, nil
	}
	cur, err := s.lifeline(ctx, vm)
	if err != nil {
		return false, err
	}
	return !runsOn(cur, r.Started), nil
}

// agentSocket is the agent a machine made forwarding starts with: none
// when its record or c says so (either is enough); else devvm's own when c
// names it, else the user's (SSH_AUTH_SOCK, read with getenv), and
// without one the start is refused.
func agentSocket(vm string, r record, c backend.Confinement, getenv func(string) string) (string, error) {
	switch {
	case r.AgentOff || c.NoAgent:
		return noAgent, nil
	case c.AgentSocket != "":
		return c.AgentSocket, nil
	}
	if sock := getenv("SSH_AUTH_SOCK"); sock != "" {
		return sock, nil
	}
	return "", msg.SmolvmNoAgent(vm)
}
