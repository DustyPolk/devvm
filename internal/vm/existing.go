package vm

import (
	"bytes"
	"context"
	"io"
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
	// Held until its state lists the new tools, as a new VM's.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	files, changed, err := tools.VMs{Env: e.Tools}.Update(ctx, buildFn, vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), tg)
	if err == nil && changed {
		err = st.Set(KeyTools, strings.Join(files, " "))
	}
	unmaking()
	if err != nil {
		return err
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
	if err := (build.Builds{Env: builder}).WarnIfOutdated(st.Get(state.KeyChain)); err != nil {
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
	if err := e.setUpVM(ctx, b, st, t.f, conf); err != nil {
		return err
	}
	cmd, err := b.Shell(ctx, vm, t.shellDir(ctx, e.LogWriter()))
	if err != nil {
		return err
	}
	return e.Exec(cmd)
}

// shellDir is the folder in a running VM that its shell and editors open
// in: its project folder, else, for a parent folder's VM, the folder devvm
// runs in (t.sub), at the same path in the VM. A --clone VM's clone is at
// the parent's path: it's the subfolder when the clone has it, else the
// clone's top. The check's errors go to log.
func (t *found) shellDir(ctx context.Context, log io.Writer) string {
	if t.sub == "" {
		return t.f.Dir
	}
	if isClone(t.st) {
		err := t.b.Exec(ctx, t.f.Name, []string{"test", "-d", t.sub}, backend.ExecOpts{Stderr: log})
		if err != nil {
			return t.f.Dir
		}
	}
	return t.sub
}

// launch takes a VM that exists, made for this folder, through what each
// launch of it does before its setup: the folder's devcontainer.json
// compared with the one it was made from; .git checked (and on Linux, set-id
// files) unless it's a --clone VM, which mounts neither; a stopped VM's
// size and SSH agent forwarding made what its state asks for; the
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
	devcontainers(e).check(st, t.f.Dir)
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
	if err := e.applyAgent(ctx, b, st, in); err != nil {
		return backend.Confinement{}, err
	}
	if sshagent.Mode(st) == sshagent.On {
		e.checkAgent(ctx)
	}
	conf, err := e.run(ctx, b, st)
	if err == nil {
		e.warnDisk(ctx, b, vm)
	}
	return conf, err
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
	if o.NoDevcontainer {
		e.Warn(msg.NoDevcontainerFlagIgnored())
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
	if want && in.AgentFixed {
		return msg.AgentFixed(b.Name(), st.Name())
	}
	if in.Status == backend.StatusRunning {
		e.Warn(msg.AgentNextStart(st.Name()))
		return nil
	}
	return b.SetAgentForwarding(ctx, st.Name(), want)
}

// run starts the VM if it's stopped, confined as its state says, then
// checks that its network policy holds (netpolicy.SelfCheck, which stops
// it if not). A running VM that may forward an SSH agent devvm didn't
// choose is stopped (checkAgentStart). It returns the confinement the VM
// runs with. A running VM's
// proxy is kept, unless it needs starting again (Prepare); one whose
// network policy was turned off keeps it until it starts again.
func (e Launch) run(ctx context.Context, b backend.Backend, st *state.VM) (backend.Confinement, error) {
	vm := st.Name()
	if st.Get(KeyNet) == "" {
		return backend.Confinement{}, msg.VMUnfinished(vm)
	}
	conf := backend.Confinement{Net: backend.NetProxy}
	switch {
	case !netpolicy.On(st):
		conf.Net = backend.NetDirect
	case st.Get(KeyDockerLayer) != "":
		// Its state edited by hand: root in it could change its network.
		return conf, msg.NetDockerLayer(b.Name(), vm)
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
	if in.Status == backend.StatusRunning {
		if err := e.reloadAgent(ctx, b, st); err != nil {
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
	// One that was running, unless the self-check started it again, is
	// checked for an agent devvm didn't choose; after network policy's
	// check, which says more.
	if in.Status == backend.StatusRunning {
		if err := e.checkAgentStart(ctx, b, st); err != nil {
			return conf, err
		}
	}
	return started, nil
}

// reloadAgent loads the keys whose lifetime ran out into devvm's agent
// again for a running VM that runs with it (runningAgent); one that
// starts gets them in start.
func (e Launch) reloadAgent(ctx context.Context, b backend.Backend, st *state.VM) error {
	mode, err := runningAgent(ctx, b, st)
	if err != nil || mode != sshagent.Devvm {
		return err
	}
	return e.Agent.Ensure(ctx)
}

// addLayers adds to the VM, in place and after asking, what it lacks of
// the image want names (agents alone go on top of the folder's
// toolchains, as for a new VM): shared tools join its list and are
// mounted on launch; a layer's script runs in it. New agents' folders are
// mounted, which restarts it. On a backend that makes VMs for the docker
// layer, adding it makes the VM one (dockerLayers), its container made
// again at the start that runs the layer's script; a script that fails
// makes it one as it was.
func (e Launch) addLayers(ctx context.Context, want string, t *found, builder *build.Env) error {
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
	mode, need, err := dockerLayers(e).inPlace(b, st, added)
	if err != nil {
		return err
	}
	if err := e.Registry.Require(b, vm, need...); err != nil {
		return err
	}
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
	// Held until its state lists the new tools, as a new VM's.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	defer unmaking()
	tg := toolTarget(b, st)
	files, err := tools.Versions{Env: e.Tools}.Files(ctx, image.ToolsOf(added), tg)
	if err != nil {
		return err
	}
	if err := e.buildTools(ctx, builder, tg, files); err != nil {
		return err
	}
	newImg := image.Normalize(img + "+" + added)
	if len(mounts) > 0 || mode != "" {
		if err := b.Stop(ctx, vm); err != nil {
			return err
		}
	}
	if len(mounts) > 0 {
		if err := b.SetAgentDirs(ctx, vm, e.AgentMounts(newImg)); err != nil {
			return err
		}
	}
	if err := dockerLayers(e).set(ctx, b, st, mode); err != nil {
		return err
	}
	// The new image is the VM's while its layers' scripts run, so its proxy
	// allows their hosts too; it's set back when one fails, and a VM made
	// one for the docker layer is made one as it was.
	if err := st.Set(state.KeyImage, newImg); err != nil {
		return err
	}
	if err := e.runLayers(ctx, b, st, builder, missing); err != nil {
		if rerr := st.Set(state.KeyImage, img); rerr != nil {
			return rerr
		}
		if rerr := dockerLayers(e).unset(ctx, b, st, mode); rerr != nil {
			return rerr
		}
		return err
	}
	if len(files) > 0 {
		if err := st.Set(KeyTools, strings.Join(append(strings.Fields(st.Get(KeyTools)), files...), " ")); err != nil {
			return err
		}
	}
	e.Info(msg.VMIsNow(newImg, vm))
	return nil
}

// runLayers starts the VM as its state says, then runs the scripts of the
// layers among names in it, in order.
func (e Launch) runLayers(ctx context.Context, b backend.Backend, st *state.VM, builder *build.Env, names []string) error {
	vm := st.Name()
	if _, err := e.run(ctx, b, st); err != nil {
		return err
	}
	for _, l := range names {
		if image.IsTool(l) {
			continue
		}
		data, err := builder.LayerData(l)
		if err != nil {
			return err
		}
		e.Info(msg.Adding(l))
		e.LogNote()
		out := e.LogWriter()
		if err := b.Exec(ctx, vm, []string{"bash", "-s"}, backend.ExecOpts{Stdin: bytes.NewReader(data), Stdout: out, Stderr: out}); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return backend.Logged(msg.AddLayerFailed(l, vm))
		}
	}
	return nil
}

// wantedImage is the image want names for this folder, as for a new VM,
// without saying what it detected.
func (e Launch) wantedImage(want, dir string, b *build.Env) (string, error) {
	if needsDetection(want) {
		if found := project.Detect(dir); len(found) > 0 {
			want = strings.Join(append(found, want), "+")
		}
	}
	img := image.Normalize(want)
	return img, b.CheckLayers(img)
}

// toolTarget is what a VM's tool images are built for: its base's C
// library and its backend's format.
func toolTarget(b backend.Backend, st *state.VM) tools.Target {
	return tools.Target{Libc: st.Get(KeyLibc), Format: b.ToolFormat()}
}

// buildTools builds the tool images the share lacks of files, from base's
// image as it is now, built first if it isn't.
func (e Launch) buildTools(ctx context.Context, builder *build.Env, tg tools.Target, files []string) error {
	share := tools.Share{Env: e.Tools}
	if !slices.ContainsFunc(files, func(f string) bool { return !share.Built(f) }) {
		return nil
	}
	if err := (build.Builds{Env: builder}).Build(ctx, image.Base); err != nil {
		return err
	}
	base, err := builder.Key(image.Base)
	if err != nil {
		return err
	}
	return share.Build(ctx, builder.Review, builder.Backend, base, tg, files)
}
