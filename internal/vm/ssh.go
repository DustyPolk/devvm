package vm

import (
	"context"
	"os/exec"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// SSH gets the folder's VM ready for ssh and editors (reach), then says
// the name they reach it by and its folder.
func (e *Env) SSH(ctx context.Context) error {
	host, t, err := e.reach(ctx)
	if err != nil {
		return err
	}
	e.Say(msg.SSHHostInfo(t.f.Dir, host))
	return nil
}

// Code gets the folder's VM ready for ssh and editors (reach), then opens
// its folder in VS Code over Remote-SSH, in devvm's place, with the
// environment devvm started with: the SSH agent devvm's own connections
// use isn't the editor's.
func (e *Env) Code(ctx context.Context) error {
	host, t, err := e.reach(ctx)
	if err != nil {
		return err
	}
	path, err := exec.LookPath("code")
	if err != nil {
		return msg.NoCodeCommand(host)
	}
	return e.Exec(backend.Command{
		Path: path,
		Args: []string{"code", "--remote", "ssh-remote+" + host, t.f.Dir},
		Env:  e.Environ,
	})
}

// reach launches the folder's VM as devvm does without its shell
// (launch), sets it up, checks its backend says how ssh reaches it, and
// makes sure ~/.ssh/config includes devvm's ssh config, asking first. It
// returns the name ssh reaches it by.
func (e *Env) reach(ctx context.Context) (string, *found, error) {
	t, err := e.existing(ctx, "")
	if err != nil {
		return "", nil, err
	}
	if err := backend.Require(t.b, t.f.Name, backend.FeatureSSH); err != nil {
		return "", nil, err
	}
	conf, err := e.launch(ctx, t)
	if err != nil {
		return "", nil, err
	}
	if err := e.setUpVM(ctx, t.b, t.st, t.f, conf); err != nil {
		return "", nil, err
	}
	// A VM running since a start without its ssh (an emulation not yet
	// let through then) has none to reach.
	if _, err := t.b.SSH(ctx, t.f.Name); err != nil {
		return "", nil, err
	}
	c := e.sshConfig()
	host := c.Host(ctx, t.f.Name)
	if _, err := c.EnsureInclude(e.Home); err != nil {
		return "", nil, err
	}
	return host, t, nil
}
