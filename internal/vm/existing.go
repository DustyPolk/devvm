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
	"github.com/w3cj/devvm/internal/ui"
)

// open opens a shell in a VM that exists, made for this folder. An image
// named adds what the VM lacks of it, in place, after asking. Flags for
// new VMs are ignored, with a warning. On the way it warns about images
// and tools newer than the VM's, matches its agent folder mounts to the
// host's, checks .git (and on Linux, set-id files) unless it's a --clone
// VM (--clone on one that mounts the folder is refused), and makes a stopped
// VM's backend config match the size and ports its state asks for; then
// it starts the VM if it's stopped, checks its network policy, sets it
// up and opens the shell, as for a new VM.
func (e *Env) open(ctx context.Context, o Options, t *found) error {
	vm, st, b := t.f.Name, t.st, t.b
	if !st.Exists() || st.Get(KeyNet) == "" {
		return msg.VMUnfinished(vm)
	}
	if o.Clone && !isClone(st) {
		return msg.CloneNeedsNewVM(vm)
	}
	if t.flagIgnored {
		ui.Warn(msg.BackendFlagIgnored(b.Name(), vm))
	}
	if o.From != "" && o.From != st.Get(state.KeyFrom) {
		ui.Warn(msg.FromIgnored(vm))
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
	if err := e.Tools.WarnNewer(ctx, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), st.Get(KeyLibc)); err != nil {
		return err
	}
	if err := e.Tools.RefreshDaily(); err != nil {
		return err
	}
	if err := UpdateAgentDirs(ctx, b, st, e.Home); err != nil {
		return err
	}
	warnIgnored(o)
	in, err := b.Inspect(ctx, vm)
	if err != nil {
		return err
	}
	running := in.Status == backend.StatusRunning
	// A clone mounts nothing of the folder's.
	if !isClone(st) {
		if err := e.Protector(t.f.Dir).Check(ctx, st, running); err != nil {
			return err
		}
		e.checkSetID(t.f.Dir)
	}
	if err := applySize(ctx, b, st, in); err != nil {
		return err
	}
	if err := applyPorts(ctx, b, st, running); err != nil {
		return err
	}
	if err := applyAgent(ctx, b, st, in); err != nil {
		return err
	}
	if sshagent.Mode(st) == sshagent.On {
		e.checkAgent(ctx)
	}
	conf, err := e.run(ctx, b, st)
	if err != nil {
		return err
	}
	cmd, err := e.setUp(ctx, b, st, t.f, conf)
	if err != nil {
		return err
	}
	return e.Exec(cmd)
}

// warnIgnored warns about the flags only new VMs take.
func warnIgnored(o Options) {
	if o.NetworkPolicy || o.NoNetworkPolicy {
		ui.Warn(msg.NetFlagIgnored())
	}
	if o.NoGitProtect {
		ui.Warn(msg.GitProtectFlagIgnored())
	}
	if o.Size != (Size{}) {
		ui.Warn(msg.SizeFlagsIgnored())
	}
	if o.Ports != "" {
		ui.Warn(msg.PortsFlagIgnored())
	}
	if o.SSHAgent {
		ui.Warn(msg.SSHAgentFlagIgnored())
	}
}

// applyAgent makes a stopped VM's backend forward an SSH agent when its
// mode says it does, and not otherwise. A running VM's waits for its next
// start.
func applyAgent(ctx context.Context, b backend.Backend, st *state.VM, in backend.Instance) error {
	want := sshagent.Forwards(sshagent.Mode(st))
	if want {
		if err := backend.Require(b, st.Name(), backend.FeatureAgentForwarding); err != nil {
			return err
		}
	}
	if in.AgentForwarding == want {
		return nil
	}
	if in.Status == backend.StatusRunning {
		ui.Warn(msg.AgentNextStart(st.Name()))
		return nil
	}
	return b.SetAgentForwarding(ctx, st.Name(), want)
}

// checkSetID names set-id files in the folder on Linux hosts, where they
// reach the host (macOS's virtiofs drops the bits).
func (e *Env) checkSetID(dir string) {
	if e.OS != "linux" {
		return
	}
	if files := project.SetIDFiles(dir); len(files) > 0 {
		var shown []string
		for _, p := range files {
			shown = append(shown, msg.Shown(p))
		}
		ui.Warn(msg.SetidFiles(strings.Join(shown, " ")))
	}
}

// run starts the VM if it's stopped, confined as its state says, then
// checks that its network policy holds (netpolicy.SelfCheck, which stops
// it if not). It returns the confinement the VM runs with. A running VM's
// proxy is kept, unless it needs starting again (Prepare); one whose
// network policy was turned off keeps it until it starts again.
func (e *Env) run(ctx context.Context, b backend.Backend, st *state.VM) (backend.Confinement, error) {
	vm := st.Name()
	if st.Get(KeyNet) == "" {
		return backend.Confinement{}, msg.VMUnfinished(vm)
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if netpolicy.On(st) {
		conf.Net = backend.NetProxy
	}
	if err := backend.Require(b, vm, conf.Features()...); err != nil {
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
		ui.Info(msg.Starting(vm))
		if err := start(ctx); err != nil {
			return conf, err
		}
	case conf.Net == backend.NetProxy:
		if started, err = e.Proxy.Prepare(ctx, st, b.Lifeline(vm), true); err != nil {
			return conf, err
		}
	default:
		// Network policy turned off while it ran confined: it stays
		// on its proxy, tokens out of it, until it starts again.
		confined, err := b.Confined(ctx, vm)
		if err != nil {
			return conf, err
		}
		if confined {
			ui.Warn(msg.NetOffNextStart(vm))
			if started, err = e.Proxy.Prepare(ctx, st, b.Lifeline(vm), true); err != nil {
				return conf, err
			}
			return started, nil
		}
	}
	if err := netpolicy.SelfCheck(ctx, b, st, start); err != nil {
		return conf, err
	}
	return started, nil
}

// applySize makes a stopped VM's size what its state asks for, where it
// differs (disks only grow). A running VM's waits for its next start.
func applySize(ctx context.Context, b backend.Backend, st *state.VM, in backend.Instance) error {
	want := Size{CPUs: st.Get(KeyCPUs), Memory: st.Get(KeyMemory), Disk: st.Get(KeyDisk)}.Backend()
	var edit backend.Size
	if want.CPUs != 0 && want.CPUs != in.Size.CPUs {
		edit.CPUs = want.CPUs
	}
	if want.MemoryGiB != 0 && want.MemoryGiB != in.Size.MemoryGiB {
		edit.MemoryGiB = want.MemoryGiB
	}
	if want.DiskGiB > in.Size.DiskGiB {
		edit.DiskGiB = want.DiskGiB
	}
	if edit == (backend.Size{}) {
		return nil
	}
	if in.Status == backend.StatusRunning {
		ui.Warn(msg.SizeNextStart(st.Name()))
		return nil
	}
	return b.SetSize(ctx, st.Name(), edit)
}

// applyPorts makes a stopped VM forward the ports its state asks for, when
// its backend config forwards others (KeyPortsSet). A running VM's wait
// for its next start.
func applyPorts(ctx context.Context, b backend.Backend, st *state.VM, running bool) error {
	want, set := orAll(st.Get(KeyPorts)), orAll(st.Get(KeyPortsSet))
	if want == set {
		return nil
	}
	if running {
		ui.Warn(msg.PortsNextStart(st.Name()))
		return nil
	}
	if err := b.SetPorts(ctx, st.Name(), PortsBackend(want)); err != nil {
		return err
	}
	return st.Set(KeyPortsSet, want)
}

func orAll(ports string) string {
	if ports == "" {
		return PortsAll
	}
	return ports
}

// addLayers adds to the VM, in place and after asking, what it lacks of
// the image want names (agents alone go on top of the folder's
// toolchains, as for a new VM): shared tools join its list and are
// mounted on launch; a layer's script runs in it. New agents' folders are
// mounted, which restarts it.
func (e *Env) addLayers(ctx context.Context, want string, t *found, builder *build.Builder) error {
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
	mounts := AgentMounts(e.Home, added)
	if !terminal() {
		return msg.NoTerminalConfirm(vm, want)
	}
	if len(mounts) > 0 {
		ui.Say(msg.RestartForMounts())
	}
	if !confirm(msg.AddInPlace(img, strings.Join(missing, " "), vm)) {
		ui.Info(msg.Keeping(img))
		return nil
	}
	libc := st.Get(KeyLibc)
	files, err := e.Tools.Files(ctx, image.ToolsOf(added), libc)
	if err != nil {
		return err
	}
	if err := e.buildTools(ctx, builder, libc, files); err != nil {
		return err
	}
	newImg := image.Normalize(img + "+" + added)
	if len(mounts) > 0 {
		if err := b.Stop(ctx, vm); err != nil {
			return err
		}
		if err := b.SetAgentDirs(ctx, vm, AgentMounts(e.Home, newImg)); err != nil {
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
		ui.Info(msg.Adding(l))
		ui.LogNote()
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
	ui.Info(msg.VMIsNow(newImg, vm))
	return nil
}

// wantedImage is the image want names for this folder, as for a new VM,
// without saying what it detected.
func (e *Env) wantedImage(want, dir string, b *build.Builder) (string, error) {
	if needsDetection(want) {
		if found := project.Detect(dir); len(found) > 0 {
			want = strings.Join(append(found, want), "+")
		}
	}
	img := image.Normalize(want)
	return img, b.CheckLayers(img)
}

// runScript runs a host script in the VM, fed to bash, its output logged.
func (e *Env) runScript(ctx context.Context, b backend.Backend, vm, path string) error {
	f, err := os.Open(path) //nolint:gosec // the user's own layer
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	out := ui.LogWriter()
	return b.Exec(ctx, vm, []string{"bash", "-s"}, backend.ExecOpts{Stdin: f, Stdout: out, Stderr: out})
}

// buildTools builds the tool images the share lacks of files, from base's
// image as it is now, built first if it isn't.
func (e *Env) buildTools(ctx context.Context, builder *build.Builder, libc string, files []string) error {
	if !slices.ContainsFunc(files, func(f string) bool { return !e.Tools.Built(f) }) {
		return nil
	}
	if err := builder.Build(ctx, image.Base); err != nil {
		return err
	}
	base, err := builder.Key(image.Base)
	if err != nil {
		return err
	}
	return e.Tools.Build(ctx, builder.Backend, base, libc, files)
}
