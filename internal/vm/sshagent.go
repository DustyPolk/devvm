package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/sshagent"
)

// defaultKeys are the keys in ~/.ssh offered for loading, in order.
var defaultKeys = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

// agentVM finds the folder's VM for devvm ssh-agent.
func (e Settings) agentVM(ctx context.Context) (*found, error) {
	return e.folderVM(ctx, msg.NoVMSSHAgent)
}

// SSHAgent returns the folder's VM's SSH agent forwarding: off, on (the
// user's agent) or devvm (devvm's own).
func (e Settings) SSHAgent(ctx context.Context) (string, error) {
	t, err := e.agentVM(ctx)
	if err != nil {
		return "", err
	}
	return sshagent.Mode(t.st), nil
}

// SetSSHAgent changes the folder's VM's SSH agent forwarding, from its
// next start: off, on (the user's agent) or devvm (devvm's own, started
// with its keys loaded now).
func (e Settings) SetSSHAgent(ctx context.Context, mode string) error {
	t, err := e.agentVM(ctx)
	if err != nil {
		return err
	}
	vm, st, b := t.f.Name, t.st, t.b
	if sshagent.Mode(st) == mode {
		e.Info(msg.AgentAlready(mode, vm))
		return nil
	}
	if sshagent.Forwards(mode) {
		if err := e.Registry.Require(b, vm, backend.FeatureAgentForwarding); err != nil {
			return err
		}
		if ok, err := t.widen(e.Env, "devvm ssh-agent "+mode); !ok {
			return err
		}
	}
	if mode == sshagent.Devvm {
		if err := e.Agent.Ensure(ctx); err != nil {
			return err
		}
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	if sshagent.Forwards(mode) != in.AgentForwarding {
		if err := e.Registry.Require(b, vm, backend.FeatureAgentToggle); err != nil {
			return err
		}
		if in.AgentFixed {
			return msg.AgentFixed(b.Name(), vm)
		}
	}
	if err := st.Set(sshagent.Key, mode); err != nil {
		return err
	}
	switch mode {
	case sshagent.On:
		e.Warn(msg.AgentOnRisk(vm))
	case sshagent.Devvm:
		e.Warn(msg.AgentDevvmRisk(vm))
	}
	if in.Status == backend.StatusRunning {
		e.Info(msg.AgentSetRunning(mode, vm))
	} else {
		e.Info(msg.AgentSetStopped(mode, vm))
	}
	return nil
}

// AgentAdd loads a key into devvm's own SSH agent, and records it.
func (e Settings) AgentAdd(ctx context.Context, file string, o sshagent.AddOptions) error {
	return e.Agent.Add(ctx, file, o)
}

// AgentRemove takes a key out of devvm's own SSH agent and its record.
func (e Settings) AgentRemove(ctx context.Context, file string) error {
	return e.Agent.Remove(ctx, file)
}

// AgentKeys writes to w the keys devvm's own SSH agent holds, then those
// added to it.
func (e Settings) AgentKeys(ctx context.Context, w io.Writer) error {
	return e.Agent.Keys(ctx, w)
}

// AgentKeyList returns the keys added to devvm's own SSH agent, each with
// whether the agent holds it, loading those it lacks first.
func (e Settings) AgentKeyList(ctx context.Context) ([]sshagent.ListedKey, error) {
	return e.Agent.KeyList(ctx)
}

// checkAgent is for a VM that forwards the user's SSH agent: forwarding
// an empty one does nothing, so when the agent has no keys and ~/.ssh
// has one, it offers to load it.
func (e Launch) checkAgent(ctx context.Context) {
	rc := e.SSHAdd(ctx, "-l")
	if rc == 0 {
		return
	}
	key := ""
	for _, k := range defaultKeys {
		p := filepath.Join(e.Dirs.Home, ".ssh", k)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			key = p
			break
		}
	}
	if key == "" {
		return
	}
	// ssh-add -l: 1 for no keys, 2 for no agent.
	if rc == 2 {
		e.Warn(msg.NoSSHAgent())
		return
	}
	args := []string{key}
	if e.OS == "darwin" {
		args = []string{"--apple-use-keychain", key}
	}
	cmd := "ssh-add " + strings.Join(args, " ")
	e.Warn(msg.SSHAgentEmpty())
	if e.Terminal() && e.Confirm(msg.LoadKey(cmd, key)) && e.SSHAdd(ctx, args...) == 0 {
		if e.OS == "darwin" {
			e.Info(msg.KeyLoadedKeychain())
		} else {
			e.Info(msg.KeyLoaded())
		}
		return
	}
	e.Warn(msg.LoadKeyWith(cmd))
}

// sshAdd runs ssh-add with args on the terminal, which it may ask a
// passphrase on, its output to out, and returns its exit code: 127 when
// it can't run.
func sshAdd(ctx context.Context, out io.Writer, args ...string) int {
	cmd := exec.CommandContext(ctx, "ssh-add", args...) //nolint:gosec // ssh-add with -l or a key's path
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, out
	if args[0] == "-l" {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	}
	err := cmd.Run()
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	if err != nil {
		return 127
	}
	return 0
}
