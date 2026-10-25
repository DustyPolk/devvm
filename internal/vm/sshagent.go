package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// defaultKeys are the keys in ~/.ssh offered for loading, in order.
var defaultKeys = []string{"id_ed25519", "id_ecdsa", "id_rsa"}

// checkAgent is for a VM that forwards the user's SSH agent: forwarding
// an empty one does nothing, so when the agent has no keys and ~/.ssh
// has one, it offers to load it.
func (e *Env) checkAgent(ctx context.Context) {
	rc := e.SSHAdd(ctx, "-l")
	if rc == 0 {
		return
	}
	key := ""
	for _, k := range defaultKeys {
		p := filepath.Join(e.Home, ".ssh", k)
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
