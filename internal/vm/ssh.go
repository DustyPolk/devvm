package vm

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/vmssh"
)

// ownSSH is how devvm's own ssh reaches a running VM, for links and a
// --clone VM's push and fetch: its backend's ssh (reviewed), with the
// VM's own key and the host keys devvm pinned for its current start
// (vmssh.KnownHostsFor), on a host loopback port (vmssh.FromTarget).
func (e *Env) ownSSH(ctx context.Context, b backend.Backend, vm string) (vmssh.SSH, error) {
	if !e.Registry.Allowed(b, backend.FeatureSSH) {
		return vmssh.SSH{}, fmt.Errorf("%s: the %s backend's ssh isn't usable", vm, b.Name())
	}
	t, err := b.SSH(ctx, vm)
	if err != nil {
		return vmssh.SSH{}, err
	}
	s, err := vmssh.FromTarget(vm, t)
	if err != nil {
		return vmssh.SSH{}, err
	}
	if s.KnownHosts, err = vmssh.KnownHostsFor(ctx, e.Dirs, b, vm); err != nil {
		return vmssh.SSH{}, err
	}
	return s, nil
}

// ShowHost gets the folder's VM ready for ssh and editors (reach), then
// says the name they reach it by and the folder to open (shellDir).
func (e Launch) ShowHost(ctx context.Context) error {
	host, t, err := e.reach(ctx)
	if err != nil {
		return err
	}
	e.Say(msg.SSHHostInfo(t.shellDir(ctx, e.LogWriter()), host))
	return nil
}

// Code gets the folder's VM ready for ssh and editors (reach), then opens
// the folder its shell would open in (shellDir) in VS Code over
// Remote-SSH, in devvm's place, with the
// environment devvm started with: the SSH agent devvm's own connections
// use isn't the editor's.
func (e Launch) Code(ctx context.Context) error {
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
		Args: []string{"code", "--remote", "ssh-remote+" + host, t.shellDir(ctx, e.LogWriter())},
		Env:  e.Environ,
	})
}

// reach launches the folder's VM as devvm does without its shell
// (launch), sets it up, checks its backend says how ssh reaches it, and
// makes sure ~/.ssh/config includes devvm's ssh config, asking first. It
// returns the name ssh reaches it by.
func (e Launch) reach(ctx context.Context) (string, *found, error) {
	t, err := ready(ctx, e)
	if err != nil {
		return "", nil, err
	}
	c := e.sshConfig()
	host := c.Host(t.f.Name)
	if _, err := c.EnsureInclude(e.Dirs.Home); err != nil {
		return "", nil, err
	}
	return host, t, nil
}

// ready launches the folder's VM as devvm does without its shell (launch),
// sets it up, which puts it in devvm's ssh config, and checks its backend
// says how ssh reaches it.
func ready(ctx context.Context, e Launch) (*found, error) {
	t, err := e.existing(ctx)
	if err != nil {
		return nil, err
	}
	if err := e.Registry.Require(t.b, t.f.Name, backend.FeatureSSH); err != nil {
		return nil, err
	}
	conf, err := e.launch(ctx, t)
	if err != nil {
		return nil, err
	}
	if err := e.setUpVM(ctx, t.b, t.st, t.f, conf); err != nil {
		return nil, err
	}
	// A VM running since a start without its ssh (an emulation not yet
	// let through then) has none to reach.
	if _, err := t.b.SSH(ctx, t.f.Name); err != nil {
		return nil, err
	}
	return t, nil
}
