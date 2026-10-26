package vm

import (
	"context"
	"os"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Restart stops the folder's VM, then opens it as Up does: what changes
// only when it starts (its mounts, size, ports) applies.
func (e Launch) Restart(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	if err := t.b.Stop(ctx, t.f.Name); err != nil {
		return err
	}
	o.Image = ""
	return e.open(ctx, o, t)
}

// Remove takes shared tools (names joined with +) out of the folder's VM,
// then opens it: they leave its image name and its list, and aren't
// mounted from its next launch (an agent's folders neither). What the VM
// installed itself stays.
func (e Launch) Remove(ctx context.Context, o Options, names string) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	img, files, err := tools.Remove(vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), image.Normalize(names))
	if err != nil {
		return err
	}
	if err := st.Set(state.KeyImage, img); err != nil {
		return err
	}
	if err := st.Set(KeyTools, strings.Join(files, " ")); err != nil {
		return err
	}
	e.Info(msg.VMIsNow(img, vm))
	o.Image = ""
	return e.open(ctx, o, t)
}

// Update checks for newer shared tools and moves the folder's VM to them,
// built first, then opens it. Nothing in its disk changes: its tools are
// mounted from the share.
func (e Launch) Update(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	builder := e.builder(t.b, st.Get(state.KeyFrom))
	tg := toolTarget(t.b, st)
	buildFn := func(ctx context.Context, files []string) error { return e.buildTools(ctx, builder, tg, files) }
	files, changed, err := tools.VMs{Env: e.Tools}.Update(ctx, buildFn, vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), tg)
	if err != nil {
		return err
	}
	if changed {
		if err := st.Set(KeyTools, strings.Join(files, " ")); err != nil {
			return err
		}
	}
	o.Image = ""
	return e.open(ctx, o, t)
}

// open opens a shell in a VM that exists, made for this folder. An image
// named adds what the VM lacks of it, in place, after asking. Flags for
// new VMs are ignored, with a warning. On the way it warns about images
// and tools newer than the VM's, matches its agent folder mounts to the
// host's (--clone on a VM that mounts the folder is refused); then it
// launches the VM (launch), sets it up and opens the shell, as for a new
// VM.
func (e Launch) open(ctx context.Context, o Options, t *found) error {
	vm, st, b := t.f.Name, t.st, t.b
	if !st.Exists() || st.Get(KeyNet) == "" {
		return msg.VMUnfinished(vm)
	}
	if o.Clone && !isClone(st) {
		return msg.CloneNeedsNewVM(vm)
	}
	if t.flagIgnored {
		e.Warn(msg.BackendFlagIgnored(b.Name(), vm))
	}
	if o.From != "" && o.From != st.Get(state.KeyFrom) {
		e.Warn(msg.FromIgnored(vm))
	}
	builder := e.builder(b, st.Get(state.KeyFrom))
	if o.Image != "" {
		if err := e.addLayers(ctx, o.Image, t, builder); err != nil {
			return err
		}
	}
	if err := builder.WarnIfOutdated(st.Get(state.KeyChain)); err != nil {
		return err
	}
	have := strings.Fields(st.Get(KeyTools))
	if err := (tools.VMs{Env: e.Tools}).WarnNewer(ctx, st.Get(state.KeyImage), have, toolTarget(b, st)); err != nil {
		return err
	}
	if err := (tools.Versions{Env: e.Tools}).RefreshDaily(); err != nil {
		return err
	}
	if err := e.UpdateAgentDirs(ctx, b, st); err != nil {
		return err
	}
	e.warnIgnored(o)
	conf, err := e.launch(ctx, t)
	if err != nil {
		return err
	}
	cmd, err := e.setUp(ctx, b, st, t.f, conf)
	if err != nil {
		return err
	}
	return e.Exec(cmd)
}

// launch takes a VM that exists, made for this folder, through what each
// launch of it does before its setup: .git checked (and on Linux, set-id
// files) unless it's a --clone VM, which mounts neither; a stopped VM's
// size, ports and SSH agent forwarding made what its state asks for; the
// user's agent checked for keys when the VM forwards it; then it's
// started if it's stopped and its network policy checked (run). It
// returns the confinement the VM runs with.
func (e Launch) launch(ctx context.Context, t *found) (backend.Confinement, error) {
	vm, st, b := t.f.Name, t.st, t.b
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return backend.Confinement{}, err
	}
	running := in.Status == backend.StatusRunning
	// A clone mounts nothing of the folder's.
	if !isClone(st) {
		if err := e.Protector(t.f.Dir).Check(ctx, st, running); err != nil {
			return backend.Confinement{}, err
		}
		e.checkSetID(t.f.Dir)
	}
	if err := e.applySize(ctx, b, st, in); err != nil {
		return backend.Confinement{}, err
	}
	if err := e.applyPorts(ctx, b, st, running); err != nil {
		return backend.Confinement{}, err
	}
	if err := e.applyAgent(ctx, b, st, in); err != nil {
		return backend.Confinement{}, err
	}
	if sshagent.Mode(st) == sshagent.On {
		e.checkAgent(ctx)
	}
	return e.run(ctx, b, st)
}

// warnIgnored warns about the flags only new VMs take.
func (e Launch) warnIgnored(o Options) {
	if o.NetworkPolicy || o.NoNetworkPolicy {
		e.Warn(msg.NetFlagIgnored())
	}
	if o.NoGitProtect {
		e.Warn(msg.GitProtectFlagIgnored())
	}
	if o.Size != (Size{}) {
		e.Warn(msg.SizeFlagsIgnored())
	}
	if o.Ports != "" {
		e.Warn(msg.PortsFlagIgnored())
	}
	if o.SSHAgent {
		e.Warn(msg.SSHAgentFlagIgnored())
	}
}

// applyAgent makes a stopped VM's backend forward an SSH agent when its
// mode says it does, and not otherwise. A running VM's waits for its next
// start.
func (e Launch) applyAgent(ctx context.Context, b backend.Backend, st *state.VM, in backend.Instance) error {
	want := sshagent.Forwards(sshagent.Mode(st))
	if want {
		if err := e.Registry.Require(b, st.Name(), backend.FeatureAgentForwarding); err != nil {
			return err
		}
	}
	if in.AgentForwarding == want {
		return nil
	}
	if err := e.Registry.Require(b, st.Name(), backend.FeatureAgentToggle); err != nil {
		return err
	}
	if in.Status == backend.StatusRunning {
		e.Warn(msg.AgentNextStart(st.Name()))
		return nil
	}
	return b.SetAgentForwarding(ctx, st.Name(), want)
}

// run starts the VM if it's stopped, confined as its state says, then
// checks that its network policy holds (netpolicy.SelfCheck, which stops
// it if not). It returns the confinement the VM runs with. A running VM's
// proxy is kept, unless it needs starting again (Prepare); one whose
// network policy was turned off keeps it until it starts again.
func (e Launch) run(ctx context.Context, b backend.Backend, st *state.VM) (backend.Confinement, error) {
	vm := st.Name()
	if st.Get(KeyNet) == "" {
		return backend.Confinement{}, msg.VMUnfinished(vm)
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if netpolicy.On(st) {
		conf.Net = backend.NetProxy
	}
	if err := e.Registry.Require(b, vm, conf.Features()...); err != nil {
		return conf, err
	}
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return conf, err
	}
	started := conf
	start := func(ctx context.Context) error {
		var err error
		started, err = e.start(ctx, b, st, conf)
		return err
	}
	// A running VM that uses devvm's agent gets the keys whose lifetime
	// ran out loaded again; one that starts does in start.
	if in.Status == backend.StatusRunning && sshagent.Mode(st) == sshagent.Devvm {
		if err := e.Agent.Ensure(ctx); err != nil {
			return conf, err
		}
	}
	switch {
	case in.Status != backend.StatusRunning:
		e.Info(msg.Starting(vm))
		if err := start(ctx); err != nil {
			return conf, err
		}
	case conf.Net == backend.NetProxy:
		// A VM with network policy doesn't run on without its proxy.
		if started, err = e.Proxy.Prepare(ctx, b, st, true); err != nil {
			return conf, e.Proxy.Unproxied(ctx, b, vm, err)
		}
	default:
		// Network policy turned off while it ran confined: it stays
		// on its proxy, tokens out of it, until it starts again.
		confined, err := b.Confined(ctx, vm)
		if err != nil {
			return conf, err
		}
		if confined {
			e.Warn(msg.NetOffNextStart(vm))
			if started, err = e.Proxy.Prepare(ctx, b, st, true); err != nil {
				return conf, e.Proxy.Unproxied(ctx, b, vm, err)
			}
			return started, nil
		}
	}
	if err := e.Proxy.SelfCheck(ctx, b, st, start); err != nil {
		return conf, err
	}
	return started, nil
}

// addLayers adds to the VM, in place and after asking, what it lacks of
// the image want names (agents alone go on top of the folder's
// toolchains, as for a new VM): shared tools join its list and are
// mounted on launch; a layer's script runs in it. New agents' folders are
// mounted, which restarts it.
func (e Launch) addLayers(ctx context.Context, want string, t *found, builder *build.Builder) error {
	vm, st, b := t.f.Name, t.st, t.b
	img := st.Get(state.KeyImage)
	wanted, err := e.wantedImage(want, t.f.Dir, builder)
	if err != nil {
		return err
	}
	have := strings.Split(img, "+")
	var missing []string
	for p := range strings.SplitSeq(wanted, "+") {
		if p != image.Base && !slices.Contains(have, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	added := strings.Join(missing, "+")
	mounts := e.AgentMounts(added)
	if !e.Terminal() {
		return msg.NoTerminalConfirm(vm, want)
	}
	if len(mounts) > 0 {
		e.Say(msg.RestartForMounts())
	}
	if !e.Confirm(msg.AddInPlace(img, strings.Join(missing, " "), vm)) {
		e.Info(msg.Keeping(img))
		return nil
	}
	tg := toolTarget(b, st)
	files, err := tools.Versions{Env: e.Tools}.Files(ctx, image.ToolsOf(added), tg)
	if err != nil {
		return err
	}
	if err := e.buildTools(ctx, builder, tg, files); err != nil {
		return err
	}
	newImg := image.Normalize(img + "+" + added)
	if len(mounts) > 0 {
		if err := b.Stop(ctx, vm); err != nil {
			return err
		}
		if err := b.SetAgentDirs(ctx, vm, e.AgentMounts(newImg)); err != nil {
			return err
		}
	}
	if _, err := e.run(ctx, b, st); err != nil {
		return err
	}
	for _, l := range missing {
		if image.IsTool(l) {
			continue
		}
		e.Info(msg.Adding(l))
		e.LogNote()
		if err := e.runScript(ctx, b, vm, builder.LayerScript(l)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return backend.Logged(msg.AddLayerFailed(l, vm))
		}
	}
	if len(files) > 0 {
		if err := st.Set(KeyTools, strings.Join(append(strings.Fields(st.Get(KeyTools)), files...), " ")); err != nil {
			return err
		}
	}
	if err := st.Set(state.KeyImage, newImg); err != nil {
		return err
	}
	e.Info(msg.VMIsNow(newImg, vm))
	return nil
}

// wantedImage is the image want names for this folder, as for a new VM,
// without saying what it detected.
func (e Launch) wantedImage(want, dir string, b *build.Builder) (string, error) {
	if needsDetection(want) {
		if found := project.Detect(dir); len(found) > 0 {
			want = strings.Join(append(found, want), "+")
		}
	}
	img := image.Normalize(want)
	return img, b.CheckLayers(img)
}

// runScript runs a host script in the VM, fed to bash, its output logged.
func (e Launch) runScript(ctx context.Context, b backend.Backend, vm, path string) error {
	f, err := os.Open(path) //nolint:gosec // the user's own layer
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	out := e.LogWriter()
	return b.Exec(ctx, vm, []string{"bash", "-s"}, backend.ExecOpts{Stdin: f, Stdout: out, Stderr: out})
}

// toolTarget is what a VM's tool images are built for: its base's C
// library and its backend's format.
func toolTarget(b backend.Backend, st *state.VM) tools.Target {
	return tools.Target{Libc: st.Get(KeyLibc), Format: b.ToolFormat()}
}

// buildTools builds the tool images the share lacks of files, from base's
// image as it is now, built first if it isn't.
func (e Launch) buildTools(ctx context.Context, builder *build.Builder, tg tools.Target, files []string) error {
	share := tools.Share{Env: e.Tools}
	if !slices.ContainsFunc(files, func(f string) bool { return !share.Built(f) }) {
		return nil
	}
	if err := builder.Build(ctx, image.Base); err != nil {
		return err
	}
	base, err := builder.Key(image.Base)
	if err != nil {
		return err
	}
	return share.Build(ctx, builder.Review, builder.Backend, base, tg, files)
}
