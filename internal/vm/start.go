package vm

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/hostsafety"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/vmssh"
)

// starter is the one way devvm starts a project VM or goes on with one
// that runs (bringUp), whichever command it's for: a launch, ssh's, a
// command that runs in it, a remake's new machine and a new VM.
type starter struct{ *Env }

// upOpts is what a start path asks of bringUp beyond what every start
// does.
type upOpts struct {
	// made: the VM was just made (creation), so its folder gets a new
	// VM's checks (hostChecks).
	made bool
	// agentDirs: its agent folder mounts are matched to the host's first
	// (Launch.updateAgentDirs), as a launch does.
	agentDirs bool
}

// bringUp takes t's VM, made and finished, to running, confined as its
// state says, in these steps: its instance read once; for a launch, its
// agent folder mounts matched to the host's, which can stop it; its
// project folder checked on the host (hostChecks); what it needs of its
// backend refused where the backend can't (requirements); the user's
// agent checked for keys when it forwards it; then a stopped VM started
// (start), or a running one gone on with as it runs (runningConfinement),
// what its next start changes said (nextStart); last, the self-check,
// which stops a VM whose network policy doesn't hold. It returns the
// confinement the VM runs with.
func (s starter) bringUp(ctx context.Context, t *found, o upOpts) (backend.Confinement, error) {
	vm, st, b := t.f.Name, t.st, t.b
	if err := refuseUnfinished(vm, st); err != nil {
		return backend.Confinement{}, err
	}
	in, err := s.instance(ctx, t, o)
	if err != nil {
		return backend.Confinement{}, err
	}
	mountsLater := false
	if o.agentDirs {
		if in, mountsLater, err = Launch(s).updateAgentDirs(ctx, b, st, in); err != nil {
			return backend.Confinement{}, err
		}
	}
	running := in.Status == backend.StatusRunning
	if err := s.hostChecks(ctx, t, running, o.made); err != nil {
		return backend.Confinement{}, err
	}
	need, err := startNeeds(st, in)
	if err != nil {
		return backend.Confinement{}, err
	}
	if err := s.refuse(b, vm, requirements(need)); err != nil {
		return backend.Confinement{}, err
	}
	if sshagent.Mode(st) == sshagent.On {
		Launch(s).checkAgent(ctx)
	}
	conf := confinementFor(st)
	start := func(ctx context.Context) error {
		var err error
		conf, err = s.start(ctx, b, st, confinementFor(st))
		return err
	}
	if running {
		if err := s.reloadAgent(ctx, b, st); err != nil {
			return conf, err
		}
		if conf, err = s.runningConfinement(ctx, b, st); err != nil {
			return conf, err
		}
		s.nextStart(st, in, conf, mountsLater)
	} else {
		s.Info(msg.Starting(vm))
		if err := start(ctx); err != nil {
			return conf, err
		}
	}
	if err := s.Proxy.SelfCheck(ctx, b, st, start); err != nil {
		return conf, err
	}
	return conf, nil
}

// instance is t's VM as bringUp starts from it: its backend's answer,
// asked once, or for a VM just made (o.made), stopped, made as its state
// asks for its agent forwarding (creation.plan's spec).
func (s starter) instance(ctx context.Context, t *found, o upOpts) (backend.Instance, error) {
	if o.made {
		return backend.Instance{Name: t.f.Name, Status: backend.StatusStopped, AgentForwarding: sshagent.Forwards(sshagent.Mode(t.st))}, nil
	}
	return t.b.Inspect(ctx, t.f.Name)
}

// confinementFor is the confinement a VM starts with, as its state says:
// on its proxy with network policy, else direct.
func confinementFor(st *state.VM) backend.Confinement {
	if netpolicy.On(st) {
		return backend.Confinement{Net: backend.NetProxy}
	}
	return backend.Confinement{Net: backend.NetDirect}
}

// runningConfinement is the confinement a running VM runs with: its
// proxy's, made ready (Prepare), when it runs on one, network policy
// turned off while it ran confined included, as it stays on its proxy,
// tokens out of it, until it starts again; else direct. A VM with network
// policy whose proxy can't start is stopped (Unproxied): it never runs on
// without it.
func (s starter) runningConfinement(ctx context.Context, b backend.Backend, st *state.VM) (backend.Confinement, error) {
	vm := st.Name()
	if !netpolicy.On(st) {
		run, err := b.Running(ctx, vm)
		if err != nil || !run.Confined {
			return backend.Confinement{Net: backend.NetDirect}, err
		}
	}
	c, err := s.Proxy.Prepare(ctx, b, st, true)
	if err != nil {
		return c, s.Proxy.Unproxied(ctx, b, vm, err)
	}
	return c, nil
}

// nextStart says what a running VM gets only at its next start, each
// where it differs from what its state asks for: its size, its SSH agent
// forwarding, its agent folder mounts (mountsLater: the user didn't
// restart it), and network policy turned off while it ran confined
// (conf, as it runs).
func (s starter) nextStart(st *state.VM, in backend.Instance, conf backend.Confinement, mountsLater bool) {
	vm := st.Name()
	if pendingSize(st, in.Size) != (backend.Size{}) {
		s.Warn(msg.SizeNextStart(vm))
	}
	if in.AgentForwarding != sshagent.Forwards(sshagent.Mode(st)) {
		s.Warn(msg.AgentNextStart(vm))
	}
	if mountsLater {
		s.Warn(msg.MountsNextStart(vm))
	}
	if conf.Net == backend.NetProxy && !netpolicy.On(st) {
		s.Warn(msg.NetOffNextStart(vm))
	}
}

// needs is what a VM asks of its backend, which requirements lists: each
// field's zero asks nothing.
type needs struct {
	spec  *backend.VMSpec // a new machine's: what making it needs
	net   backend.Net     // its confinement's network
	ports backend.Ports   // the ports devvm's forwarder forwards from its backend's ssh
	img   image.Name      // its image, for its layers
	agent bool            // it forwards an SSH agent
	// toggle: its agent forwarding is turned on or off on a machine that
	// has it the other way, or it was made ready for that (an emulation's
	// sshagent.Made.Toggle), which each start relies on.
	toggle bool
	ssh    bool // a --clone VM's clone, copied in and fetched over ssh
}

// requirements lists the backend features n asks for, each once, in the
// order a refusal names the first: the one list every check of what a
// VM needs uses (creation.plan, remaking.check, bringUp, devvm ssh-agent,
// devvm ports).
func requirements(n needs) []backend.Feature {
	var fs []backend.Feature
	if n.spec != nil {
		fs = append(fs, n.spec.Features()...)
	}
	fs = append(fs, n.ports.Features()...)
	fs = append(fs, backend.Confinement{Net: n.net}.Features()...)
	fs = append(fs, backend.ImageFeatures(n.img)...)
	if n.agent {
		fs = append(fs, backend.FeatureAgentForwarding)
	}
	if n.toggle {
		fs = append(fs, backend.FeatureAgentToggle)
	}
	if n.ssh {
		fs = append(fs, backend.FeatureSSH)
	}
	var out []backend.Feature
	for _, f := range fs {
		if !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// startNeeds is what a made VM needs of its backend to start, or go on,
// as in, its instance, is: its state's network policy and ports, its SSH
// agent forwarding, and agent-toggle when its forwarding is to change, or
// a stopped one was made ready for it.
func startNeeds(st *state.VM, in backend.Instance) (needs, error) {
	p, err := ports.Of(st)
	if err != nil {
		return needs{}, err
	}
	forward, err := ports.Forwarding(p.Forward)
	if err != nil {
		return needs{}, err
	}
	a, err := sshagent.Of(st)
	if err != nil {
		return needs{}, err
	}
	agent := sshagent.Forwards(sshagent.Mode(st))
	return needs{
		net: confinementFor(st).Net, ports: forward, agent: agent,
		toggle: in.AgentForwarding != agent || a.Made.Toggle && in.Status != backend.StatusRunning,
	}, nil
}

// refuse refuses the first of fs that b doesn't let through now, with
// Require's refusal. A made VM got each when it was made or changed,
// which Require warned of then, so one let through warns nothing.
func (s starter) refuse(b backend.Backend, vm string, fs []backend.Feature) error {
	for _, f := range fs {
		if !s.Registry.Allowed(b, f) {
			return s.Registry.Require(b, vm, f)
		}
	}
	return nil
}

// hostChecks is what devvm checks of a VM's project folder on the host,
// unless it's a --clone VM, which mounts none of it. A new VM (made) gets
// its .git locked as its protection says (NewVM), the SSH origin it
// can't push to without an agent named, and its set-id files. A VM that
// exists gets its .git checked under the protection it ran with
// (gitprotect's Check), so what it changed meanwhile is named; then, once
// it's stopped, the protection chosen while it ran is set, and its set-id
// files are named (not while it runs: the walk is slow). devvm stop does
// the latter too.
func (s starter) hostChecks(ctx context.Context, t *found, running, made bool) error {
	if isClone(t.st) {
		return nil
	}
	p := s.Protector(t.f.Dir)
	if made {
		if err := p.NewVM(ctx, t.st); err != nil {
			return err
		}
		if !sshagent.Forwards(sshagent.Mode(t.st)) && hostsafety.SSHRemote(ctx, t.f.Dir) {
			s.Warn(msg.SSHRemoteHint(t.f.Name))
		}
		// Files an earlier VM of this folder made are named too.
		s.checkSetID(t.f.Dir)
		return nil
	}
	if err := p.Check(ctx, t.st, running); err != nil {
		return err
	}
	if running {
		return nil
	}
	if next := gitprotect.Next(t.st); next != "" {
		if err := setGitProtect(ctx, s.Env, p, t, next); err != nil {
			return err
		}
	}
	s.checkSetID(t.f.Dir)
	return nil
}

// checkSetID names set-id files in the folder on Linux hosts, where they
// reach the host (macOS's virtiofs drops the bits): those running as root
// apart.
func (s starter) checkSetID(dir string) {
	if s.OS != "linux" {
		return
	}
	var root, other []string
	for _, f := range hostsafety.SetIDFiles(dir) {
		if f.Root {
			root = append(root, f.Path)
		} else {
			other = append(other, f.Path)
		}
	}
	if len(root) > 0 {
		s.Warn(msg.SetidRootFiles(strings.Join(root, " ")))
	}
	if len(other) > 0 {
		s.Warn(msg.SetidFiles(strings.Join(other, " ")))
	}
}

// reloadAgent loads the keys whose lifetime ran out into devvm's agent
// again for a running VM that runs with it (runningAgent); one that
// starts gets them in start.
func (s starter) reloadAgent(ctx context.Context, b backend.Backend, st *state.VM) error {
	mode, err := runningAgent(ctx, b, st)
	if err != nil || mode != sshagent.Devvm {
		return err
	}
	return s.Agent.Ensure(ctx)
}

// start starts a stopped VM confined as conf says (confinementFor), and
// returns the confinement it got: bringUp's, and the self-check's when it
// starts the VM again. With network policy its proxy starts first, and
// the backend gets the proxy's confinement; a start that fails stops the
// proxy again. Without it, a proxy left from when it had it stops. Its
// host keys are read again once it runs: a VM whose backend config
// changed can get new ones when it boots.
func (s starter) start(ctx context.Context, b backend.Backend, st *state.VM, conf backend.Confinement) (backend.Confinement, error) {
	vm := st.Name()
	if err := vmssh.ForgetHostKeys(s.Dirs, vm); err != nil {
		return conf, err
	}
	// A link session of its last start's, which would end on its own: the
	// launch starts one for this start.
	linkRuns(s).stop(ctx, vm)
	// The VM's backend forwards the agent it gets when it starts: devvm's
	// own, with the keys it lacks loaded, for a VM that uses it; none for
	// one that uses none.
	mode := sshagent.Mode(st)
	conf.NoAgent = !sshagent.Forwards(mode)
	if mode == sshagent.Devvm {
		if err := s.Agent.Ensure(ctx); err != nil {
			return conf, err
		}
		conf.AgentSocket = s.Agent.Socket()
	}
	// devvm's own connections carry the agent of the start they're on.
	s.useAgent(mode)
	conf, err := s.confine(ctx, b, st, conf)
	if err != nil {
		return conf, err
	}
	// Tools pointed at the CAs devvm adds, with network policy or
	// without, whether there are any or not.
	conf.Env = trust.GuestEnv(conf.Env)
	// ssh for editors, where the backend has it (an emulation once it's
	// reviewed): a VM without it starts all the same.
	conf.SSH = s.Registry.Require(b, vm, backend.FeatureSSH) == nil
	if conf.Net == backend.NetProxy {
		defer s.AddCleanup(undoStart(s.Env, b, vm))()
	}
	// What its state asks of it, which the start makes it where it
	// differs: its size, its agents' folders on the host now, its agent
	// forwarding.
	agentDirs, _ := agentMounts(s.Dirs.Home, tools.Names(st))
	want := backend.Want{
		Size: sectionOf[Size](st, sizeSection).Backend(), AgentDirs: agentDirs,
		AgentForwarding: sshagent.Forwards(mode),
	}
	if err := b.Start(ctx, vm, want, conf); err != nil {
		if conf.Net == backend.NetProxy {
			s.Proxy.Stop(vm)
		}
		return conf, err
	}
	// Its ports, once it runs: its state's, through devvm's forwarder.
	s.forwards().Start(ctx, b, st)
	// The agent it started with, until its next start: its ssh config
	// entry's and devvm's shell's. One whose start can't be told apart
	// gets none recorded, and follows its choice.
	run, err := b.Running(ctx, vm)
	if err != nil {
		run.ID = ""
	}
	return conf, sshagent.RecordStart(ctx, st, mode, run.ID)
}

// confine is start's network: with network policy, the proxy's
// confinement, its proxy started and the VM's links checked, with the
// agent conf chose; without it, conf, its proxy stopped and its ssh port
// recorded, so other VMs' forwarders never take it before its backend
// binds it.
func (s starter) confine(ctx context.Context, b backend.Backend, st *state.VM, conf backend.Confinement) (backend.Confinement, error) {
	vm := st.Name()
	if conf.Net != backend.NetProxy {
		// It gets the shell's proxy settings alone: config.toml's
		// password never goes into a VM.
		if conf.Net == backend.NetDirect && s.Proxy.Company().ConfigOnly() {
			s.Warn(msg.ProxyNeedsPolicy(s.Dirs.ConfigFile(), vm))
		}
		if err := s.Proxy.Unconfined(ctx, vm); err != nil {
			return conf, err
		}
		sp, err := s.Proxy.Ports().SSH(ctx, st, false)
		if err != nil {
			return conf, err
		}
		conf.SSHPort = sp
		return conf, nil
	}
	p, err := s.Proxy.Prepare(ctx, b, st, false)
	if err != nil {
		return conf, err
	}
	p.AgentSocket, p.NoAgent = conf.AgentSocket, conf.NoAgent
	if err := syncer(s).checkStartLinks(st); err != nil {
		s.Proxy.Stop(vm)
		return conf, err
	}
	return p, nil
}

// undoStart is a start's cleanup for a VM with network policy: an
// interrupt can leave it running with its backend's start still waiting
// for it, so without the record of its confined start (Run.Confined), and
// it stops the VM and its proxy. The next launch starts both again,
// confined.
func undoStart(e *Env, b backend.Backend, vm string) func(context.Context) {
	return func(ctx context.Context) {
		if !e.Interrupted() {
			return
		}
		e.Info(msg.InterruptedStart(vm))
		if err := b.Stop(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			e.Warn(msg.StopFailed(vm))
		}
		e.Proxy.Stop(vm)
		e.Forwarders.Stop(vm)
	}
}
