package docker

import (
	"context"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/vmssh"
)

// SSH agent forwarding (agent-forwarding), and turning it on or off later
// (agent-toggle), are emulated, and reviewed: no socket is mounted into a
// container, and none of its processes has an agent, but the ssh
// sessions that forward one. Those are devvm's own and its ssh config's,
// each choosing as the VM's choice says, over the VM's ssh (ssh.go):
//
//   - devvm's shell is an ssh session once ssh runs (Shell), forwarding,
//     when its start forwards one (record.Agent), the agent devvm's
//     SSH_AUTH_SOCK names, which devvm sets from the choice the VM's
//     current start was made with: the user's, devvm's own, or none
//     (unset);
//   - devvm's ssh config's entry, for editors, ssh and git, forwards the
//     agent of that same choice (ForwardAgent and IdentityAgent), an
//     explicit no included, as it does for Lima;
//   - devvm's own other connections (its exec, its forwarder) forward
//     none.
//
// So a VM's forwarding needs nothing set when it's made: it's on or off
// in the record, for devvm ssh-agent to show. A change reaches devvm's
// shell and the ssh config's entry at the VM's next start, as on Lima
// (StartID tells starts apart).

// SetAgentForwarding turns a stopped VM's SSH agent forwarding on or off,
// from its next start: its ssh sessions forward the agent that start
// chooses (Confinement's), or none.
func (d *Docker) SetAgentForwarding(ctx context.Context, vm string, on bool) error {
	_, r, err := d.stopped(ctx, vm)
	if err != nil || r.Forwarding == on {
		return err
	}
	r.Forwarding = on
	return d.writeRecord(vm, r)
}

// sessionAgent is whether a start's ssh sessions forward an agent
// (record.Agent, vmssh.SessionArgs's): none, or the one devvm's
// SSH_AUTH_SOCK names when it opens the shell (vmssh.AgentUser), which
// devvm sets from the choice the start was made with: the user's, devvm's
// own, or none.
func sessionAgent(r record, c backend.Confinement) string {
	if !r.Forwarding || c.NoAgent {
		return ""
	}
	return vmssh.AgentUser
}
