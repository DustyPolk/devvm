package smolvm

import (
	"bytes"
	"context"
	"maps"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
)

// Start starts a stopped machine with the network c asks for, then what
// base's services do at boot on Lima (services.sh): smolvm runs no init
// system. A running machine is left as it is.
//
// Its network and environment are set while it's stopped, where they
// differ: NetDirect and NetInternet are smolvm's own network, which reaches
// the internet but not the host's loopback; NetNone is none. NetProxy is
// refused (egress-proxy). A machine that forwards an SSH agent forwards
// c.AgentSocket, or the user's (SSH_AUTH_SOCK).
func (s *Smolvm) Start(ctx context.Context, vm string, c backend.Confinement) error {
	if c.Net == backend.NetProxy {
		return msg.BackendLacks(Name, string(backend.FeatureEgressProxy), vm)
	}
	m, err := s.inspect(ctx, vm)
	if err != nil {
		return notFound(vm, err)
	}
	if m.status() == backend.StatusRunning {
		return nil
	}
	r, err := s.readRecord(vm)
	if err != nil {
		return err
	}
	var opts []string
	if want := c.Net != backend.NetNone; m.Network != want {
		if want {
			opts = append(opts, "--net")
		} else {
			opts = append(opts, "--no-net")
		}
	}
	if !maps.Equal(r.Env, c.Env) && (len(r.Env) > 0 || len(c.Env) > 0) {
		for k := range r.Env {
			if _, ok := c.Env[k]; !ok {
				opts = append(opts, "--remove-env", k)
			}
		}
		for _, kv := range sortedEnv(c.Env) {
			opts = append(opts, "-e", kv)
		}
	}
	if err := s.update(ctx, vm, opts...); err != nil {
		return err
	}
	if !maps.Equal(r.Env, c.Env) && (len(r.Env) > 0 || len(c.Env) > 0) {
		r.Env = maps.Clone(c.Env)
		if err := s.writeRecord(vm, r); err != nil {
			return err
		}
	}
	cmd := s.smolvm(ctx, "machine", "start", "--name", vm)
	if r.Forwarding {
		sock := c.AgentSocket
		if sock == "" {
			sock = s.Getenv("SSH_AUTH_SOCK")
		}
		if sock == "" {
			return msg.SmolvmNoAgent(vm)
		}
		cmd.Env = setEnv(cmd.Env, "SSH_AUTH_SOCK="+sock)
	} else {
		cmd.Env = setEnv(cmd.Env, "SSH_AUTH_SOCK")
	}
	if err := s.Runner.Run(ctx, msg.Starting(vm), cmd); err != nil {
		return backend.Logged(msg.StartFailed(vm))
	}
	return s.services(ctx, vm)
}

// services runs services.sh in a running machine; a failure stops it.
func (s *Smolvm) services(ctx context.Context, vm string) error {
	cmd := s.asRoot(ctx, vm)
	cmd.Stdin = bytes.NewReader(servicesScript)
	if err := s.Runner.Run(ctx, msg.M{}, cmd); err != nil {
		if ctx.Err() == nil {
			_ = s.stop(ctx, vm)
		}
		return backend.Logged(msg.StartFailed(vm))
	}
	return nil
}

// Confined is always false: smolvm has no egress-proxy yet, so Start never
// starts a machine confined.
func (s *Smolvm) Confined(context.Context, string) (bool, error) { return false, nil }

// Lifeline is the pid file of the process that runs the machine, in
// smolvm's folder for it: its first line is the pid, its second the start
// time. "" when smolvm can't say where that is.
func (s *Smolvm) Lifeline(vm string) string {
	out, err := s.Runner.Output(s.smolvm(context.Background(), "machine", "data-dir", "--name", vm))
	dir := strings.TrimSpace(string(out))
	if err != nil || !filepath.IsAbs(dir) {
		return ""
	}
	return filepath.Join(dir, "agent.pid")
}
