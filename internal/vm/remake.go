package vm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/vmname"
)

// RemakeOptions are the flags of a command that makes the folder's VM
// again (devvm layer add, devvm layer rm, devvm update).
type RemakeOptions struct {
	Force    bool // --force: no "make it again?" asked, which needs a terminal otherwise
	NoExport bool // --no-export: the agents' history isn't exported first
	// CleanHome (--clean-home): the new disk has the image's home folder,
	// not the VM's.
	CleanHome bool
}

// remakeInVM is a VM's remake section, while devvm makes it again: set
// before its old machine stops, and gone once the new one has started
// with its home folder. While it's set, the next devvm in the folder
// finishes the remake (remaking.finishPending), and other commands on
// the VM refuse (remake_pending).
type remakeInVM struct {
	Image string `json:"image,omitempty"` // the image it's made again from
	// Command is the command that made it again, as typed (devvm layer
	// add desktop), which its messages name.
	Command  string `json:"command,omitempty"`
	Home     bool   `json:"home,omitempty"`      // its home folder is kept (paths.VM.Home holds it until the new disk does)
	NoExport bool   `json:"no_export,omitempty"` // --no-export
	Made     bool   `json:"made,omitempty"`      // its new machine was made, so the one by its name is no longer the old one
	// Done: its new machine has its home folder and its new image is
	// recorded, so the new machine is the VM, and only deleting the old
	// one is left.
	Done bool `json:"done,omitempty"`
}

// remakePending reports whether a remake of the VM didn't finish.
func remakePending(st *state.VM) bool {
	return sectionOf[remakeInVM](st, remakeSection).Image != ""
}

// AddLayers makes the folder's VM again from its image with layers added
// on top, in the order given (docker with desktop added is
// docker+desktop), after the user agrees (remakeTo), then opens a shell
// in it. Layers it has already are said and left as they are; when it
// has them all, nothing changes. command is the command as typed. A
// remake of it that didn't finish is finished first.
func (e Lifecycle) AddLayers(ctx context.Context, command string, layers []string, o RemakeOptions) error {
	return e.remakeLayers(ctx, command, o, func(t *found) (string, error) {
		img, had := addedImage(t.st.Image, layers)
		if len(had) > 0 {
			e.Info(msg.HasLayers(strings.Join(had, " "), t.f.Name))
		}
		return img, nil
	})
}

// RemoveLayers makes the folder's VM again from its image with layers
// taken out, the rest in their order (docker+nix+desktop without nix is
// docker+desktop), after the user agrees (remakeTo), then opens a shell
// in it. A layer it lacks is said and left out, and one that isn't a
// layer at all is refused (CheckLayers); when it lacks them all,
// nothing changes. command is the command as typed. A remake of it that
// didn't finish is finished first.
func (e Lifecycle) RemoveLayers(ctx context.Context, command string, layers []string, o RemakeOptions) error {
	return e.remakeLayers(ctx, command, o, func(t *found) (string, error) {
		img, lacks := removedImage(t.st.Image, layers)
		if err := CheckLayers(lacks, e.builder(t.b, t.st.From).Layers()[1:]); err != nil {
			return "", err
		}
		if len(lacks) > 0 {
			e.Info(msg.LacksLayers(strings.Join(lacks, " "), t.f.Name))
		}
		return img, nil
	})
}

// remakeLayers makes the folder's VM again from the image that layers
// works out from it (devvm layer add's, devvm layer rm's), after the user
// agrees (remakeTo). When that's the image it has, nothing changes, but
// a VM whose remake was just finished gets its shell.
func (e Lifecycle) remakeLayers(ctx context.Context, command string, o RemakeOptions, layers func(*found) (string, error)) error {
	t, resumed, release, err := e.toRemake(ctx, command)
	if err != nil || t == nil {
		return err
	}
	defer release()
	img, err := layers(t)
	if err != nil {
		return err
	}
	if img == t.st.Image {
		if resumed {
			return Launch(e).open(ctx, Options{}, t)
		}
		return nil
	}
	return e.remakeTo(ctx, t, img, command, o)
}

// Update brings the folder's VM up to date, then opens a shell in it.
// First it moves the VM to any newer shared tools (Tools.update), which
// needs no remake, as they're mounted. Then, when its own image or one
// it's built on changed since the VM was made (outdated:
// Builds.Outdated, the warning a launch gives), it says which changed
// and makes the VM again from that image as it builds now, after the
// user agrees (agreed). Without --force or a terminal, an outdated VM is
// refused before anything changes. A VM with the newest tools that isn't
// outdated is said to be current, and nothing changes. command is the
// command as typed. A remake of it that didn't finish is finished first.
func (e Lifecycle) Update(ctx context.Context, command string, o RemakeOptions) error {
	t, resumed, release, err := e.toRemake(ctx, command)
	if err != nil || t == nil {
		return err
	}
	defer release()
	vm, st := t.f.Name, t.st
	changed, err := (build.Builds{Env: e.builder(t.b, st.From)}).Outdated(st.Chain)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		e.Info(msg.VMChanged(strings.Join(changed, ", "), vm))
		if err := e.canRemake(t, st.Image, command, o); err != nil {
			return err
		}
	}
	newer, err := Tools(e).update(ctx, t)
	if err != nil {
		return err
	}
	switch {
	case len(changed) > 0 && e.agreed(t, st.Image, o):
		return e.remakeNow(ctx, t, st.Image, command, o)
	case newer || resumed:
		// Its newer tools take effect as its shell opens, kept or not.
		return Launch(e).open(ctx, Options{}, t)
	case len(changed) == 0:
		e.Info(msg.VMCurrent(st.Image, vm))
	}
	return nil
}

// addedImage is the image img makes with layers added on top, in the
// order given (inOrder), and those of layers img has already.
func addedImage(img string, layers []string) (string, []string) {
	was := image.LayersOf(img)
	var had []string
	for _, l := range layers {
		if slices.Contains(was, l) && !slices.Contains(had, l) {
			had = append(had, l)
		}
	}
	return image.Of(inOrder(was, slices.Concat(was, layers))), had
}

// removedImage is the image img makes with layers taken out, the rest in
// their order (base when none is left), and those of layers img lacks.
func removedImage(img string, layers []string) (string, []string) {
	was := image.LayersOf(img)
	var lacks []string
	for _, l := range layers {
		if !slices.Contains(was, l) && !slices.Contains(lacks, l) {
			lacks = append(lacks, l)
		}
	}
	return image.Of(slices.DeleteFunc(was, func(l string) bool { return slices.Contains(layers, l) })), lacks
}

// toRemake finds the folder's VM for a command that makes it again
// (command, as typed), and finishes a remake of it that didn't
// (resumed). A parent folder's VM is made again only after asking in a
// terminal, and never without one; nil when the user declines. It
// refuses a VM that's missing, made for another folder, unfinished, or
// a --clone VM, whose clone is on its disk. Once it has the VM, it holds
// the VM's remake lock (hold) until release.
func (e Lifecycle) toRemake(ctx context.Context, command string) (t *found, resumed bool, release func(), err error) {
	if t, err = e.find(ctx); err != nil {
		return nil, false, nil, err
	}
	vm := t.f.Name
	if t.sub != "" {
		if !e.Terminal() {
			return nil, false, nil, msg.RemakeParentVM(command, t.f.Dir, vm)
		}
		if !e.Ask(msg.RemakeParentVMAsk(t.f.Dir, vm)) {
			return nil, false, nil, nil
		}
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, false, nil, err
	}
	if release, err = remaking(e).hold(ctx, t); err != nil {
		return nil, false, nil, err
	}
	if resumed, err = e.refuseRemake(ctx, t); err != nil {
		release()
		return nil, false, nil, err
	}
	return t, resumed, release, nil
}

// refuseRemake finishes a remake of t's VM that didn't (resumed), then
// refuses a VM that's missing, unfinished, or a --clone VM.
func (e Lifecycle) refuseRemake(ctx context.Context, t *found) (resumed bool, err error) {
	vm := t.f.Name
	if remakePending(t.st) {
		if err := remaking(e).finishPending(ctx, t); err != nil {
			return false, err
		}
		resumed = true
	}
	switch {
	case !t.exists:
		return false, msg.NoVM(t.f.Dir)
	case !t.st.Finished():
		return false, msg.VMUnfinished(vm)
	case isClone(t.st):
		return false, msg.RemakeClone(vm)
	}
	return resumed, nil
}

// hold takes t's VM's remake lock (paths.VM.RemakeLock), which a devvm
// holds while it makes the VM again or finishes doing so, and refuses
// while another devvm holds it (remake_running). A VM without state has
// nothing to lock. Under the lock it reads the VM's state again, as
// another devvm may have made the VM again since t was found, and asks
// the backend again whether it has the VM when the remake recorded
// changed meanwhile.
func (e remaking) hold(ctx context.Context, t *found) (release func(), err error) {
	if !t.st.Exists() {
		return func() {}, nil
	}
	vm, f := t.f.Name, e.Dirs.VM(t.f.Name)
	if err := state.PrivateDir(f.Run()); err != nil {
		return nil, err
	}
	unlock, ok, err := platform.TryLockFile(f.RemakeLock())
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return nil, msg.RemakeRunning(vm)
	}
	was := sectionOf[remakeInVM](t.st, remakeSection)
	st, err := state.ReadVM(e.Dirs, vm)
	if err != nil {
		unlock()
		return nil, err
	}
	t.st = st
	if sectionOf[remakeInVM](st, remakeSection) != was {
		_, err := t.b.Inspect(ctx, vm)
		if err != nil && !errors.Is(err, backend.ErrNotFound) {
			unlock()
			return nil, err
		}
		t.exists = err == nil
	}
	return unlock, nil
}

// remakeTo makes t's VM again from img (remaking.remake), once it's
// checked that it can, and the user agrees: asked in a terminal ([y/N]),
// which --force skips, and refused without either. Then it opens a shell
// in it.
func (e Lifecycle) remakeTo(ctx context.Context, t *found, img, command string, o RemakeOptions) error {
	if err := e.canRemake(t, img, command, o); err != nil {
		return err
	}
	if !e.agreed(t, img, o) {
		return nil
	}
	return e.remakeNow(ctx, t, img, command, o)
}

// agreed reports whether the user agrees to make t's VM again from img,
// once canRemake has checked that it can: asked in a terminal ([y/N]),
// which --force skips. A no is said (kept).
func (e Lifecycle) agreed(t *found, img string, o RemakeOptions) bool {
	vm := t.f.Name
	if o.Force {
		return true
	}
	ask := msg.RemakeConfirm(img, vm)
	if o.CleanHome {
		ask = msg.RemakeConfirmCleanHome(img, vm)
	}
	if !e.Ask(ask) {
		e.Info(msg.Kept(vm))
		return false
	}
	return true
}

// canRemake refuses to make t's VM again from img when remaking.check
// does, or when there's neither --force nor a terminal to ask in.
func (e Lifecycle) canRemake(t *found, img, command string, o RemakeOptions) error {
	if err := remaking(e).check(t, img); err != nil {
		return err
	}
	if !o.Force && !e.Terminal() {
		return msg.ForceNeeded(command)
	}
	return nil
}

// remakeNow makes t's VM again from img (remaking.remake), then opens a
// shell in it.
func (e Lifecycle) remakeNow(ctx context.Context, t *found, img, command string, o RemakeOptions) error {
	if err := remaking(e).remake(ctx, t, remakeInVM{Image: img, Command: command, Home: !o.CleanHome, NoExport: o.NoExport}); err != nil {
		return err
	}
	return Launch(e).open(ctx, Options{}, t)
}

// remaking makes a VM again from an image, keeping devvm's settings for
// it and its home folder: the one remake devvm layer add, devvm layer rm
// and devvm update use.
type remaking struct{ *Env }

// check refuses, before anything runs, to make t's VM again from img
// when one of img's layers is gone, or its backend can't run what the
// VM needs with img, or the disk lacks room for a VM.
func (e remaking) check(t *found, img string) error {
	vm, b := t.f.Name, t.b
	if err := e.builder(b, t.st.From).CheckLayers(img); err != nil {
		return err
	}
	spec, err := e.spec(t)
	if err != nil {
		return err
	}
	conf := backend.Confinement{Net: backend.NetDirect}
	if netpolicy.On(t.st) {
		conf.Net = backend.NetProxy
	}
	if err := e.Registry.Require(b, vm, slices.Concat(spec.Features(), conf.Features(), backend.ImageFeatures(img))...); err != nil {
		return err
	}
	return (backend.Space{Free: e.FreeSpace}).Check(b, vm, func(f backend.Floors) uint64 { return f.VM })
}

// spec is the new machine of t's VM as its state asks for it: its
// project folder, its agents' folders, the tools share, its SSH agent
// and size, as up makes a new VM's (creation.plan); its ports are
// forwarded as it starts. Its ssh port is set as it's made.
func (e remaking) spec(t *found) (backend.VMSpec, error) {
	vm, st := t.f.Name, t.st
	spec := backend.VMSpec{
		Name: vm, Project: t.f.Dir, AgentDirs: Launch(e).AgentMounts(tools.Names(st)), Tools: e.Dirs.Tools(), ToolsAt: tools.ShareAt,
		AgentForwarding: sshagent.Forwards(sshagent.Mode(st)), Size: sectionOf[Size](st, sizeSection).Backend(),
		Env: trust.GuestEnv(nil),
	}
	spec.AgentToggle = e.Registry.Require(t.b, vm, backend.FeatureAgentToggle) == nil
	return spec, nil
}

// remake makes t's VM again from p's image, each step safe to interrupt:
// the image built first, so a failed build changes nothing; the agents'
// history exported (unless p.NoExport) and its home folder saved (unless
// it's not kept), so a failure there keeps the VM as it was; then its
// remake recorded in its state, the VM stopped and its old machine set
// aside under another name (vmname.Aside), and the new one made (finish).
// Until the new one has started with its home folder, a failure or an
// interrupt deletes it and puts the old one back as it was (undo); once
// it has, the old one is deleted.
func (e remaking) remake(ctx context.Context, t *found, p remakeInVM) error {
	vm := t.f.Name
	chain, err := e.build(ctx, t, p.Image)
	if err != nil {
		return err
	}
	if !p.NoExport && len(image.AgentsOf(tools.Names(t.st))) > 0 {
		if err := Lifecycle(e).export(ctx, t, "", msg.RemakeExportFailed(p.Command, vm)); err != nil {
			return err
		}
	}
	if p.Home {
		if err := e.ensureRunning(ctx, t); err != nil {
			return err
		}
		if err := e.saveHome(ctx, t, msg.RemakeHomeFailed(p.Command, vm)); err != nil {
			return err
		}
	}
	if err := update(ctx, t.st, remakeSection, func(r *remakeInVM) error { *r = p; return nil }); err != nil {
		return err
	}
	if err := e.setAside(ctx, t); err != nil {
		// Its old machine is as it was, stopped at most.
		return errors.Join(err, e.clear(ctx, t))
	}
	var committed atomic.Bool
	done := e.AddCleanup(e.undo(t, &committed))
	if err := e.finish(ctx, t, p, chain, &committed); err != nil {
		return err
	}
	done()
	return nil
}

// build builds img for t's VM, as a new VM's is built, and returns its
// build chain.
func (e remaking) build(ctx context.Context, t *found, img string) (chainOf, error) {
	b := e.builder(t.b, t.st.From)
	if err := (build.Builds{Env: b}).Build(ctx, img); err != nil {
		return chainOf{}, err
	}
	c, err := b.Chain(img)
	if err != nil {
		return chainOf{}, err
	}
	return chainOf{key: c.Key(), chain: c.String()}, nil
}

// chainOf is an image's build chain, as a VM is made from it (its key)
// and its state records it.
type chainOf struct{ key, chain string }

// setAside stops t's VM, what runs for it with it, and gives its machine
// its aside name (vmname.Aside), so a new one can have its name.
func (e remaking) setAside(ctx context.Context, t *found) error {
	vm := t.f.Name
	if err := t.b.Stop(ctx, vm); err != nil {
		return err
	}
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	linkRuns(e).stop(ctx, vm)
	aside, _ := vmname.Aside(vm)
	return t.b.Rename(ctx, vm, aside)
}

// finish makes t's VM's new machine from p's image (built: chain) with
// what its state records, starts it as a launch does, puts its saved
// home folder in (then committed is set), records its new image and
// deletes its old machine, which ends its remake (dropOld).
func (e remaking) finish(ctx context.Context, t *found, p remakeInVM, c chainOf, committed *atomic.Bool) error {
	vm, st, b := t.f.Name, t.st, t.b
	spec, err := e.spec(t)
	if err != nil {
		return err
	}
	if spec.SSHPort, err = e.Proxy.Ports().SSH(ctx, st, false); err != nil {
		return err
	}
	e.Info(msg.CreatingFrom(p.Image, vm))
	e.LogNote()
	if err := b.CreateVM(ctx, c.key, spec); err != nil {
		return err
	}
	t.exists = true
	if err := (creation{Launch(e)}).created(ctx, b, st, spec.AgentToggle); err != nil {
		return err
	}
	// The new machine is the one by its name now, and its
	// devcontainer.json's commands run again on its new disk.
	err = st.Update(ctx, func(st *state.VM) error {
		if err := edit(st, remakeSection, func(r *remakeInVM) error { r.Made = true; return nil }); err != nil {
			return err
		}
		return edit(st, devcontainerSection, func(d *devcontainerInVM) error { d.Made = devcontainerMade{}; return nil })
	})
	if err != nil {
		return err
	}
	if _, err := Launch(e).startChecked(ctx, t); err != nil {
		return err
	}
	if p.Home {
		if err := e.restoreHome(ctx, t); err != nil {
			return err
		}
	}
	// The new machine has its home folder, so it's the VM now: no undo
	// deletes it, and once its image is recorded, only deleting the old
	// machine is left (Done), which the next devvm finishes when this
	// one doesn't. Should recording fail, the next devvm finds the old
	// machine aside and puts it back.
	committed.Store(true)
	err = st.Update(ctx, func(st *state.VM) error {
		st.Image, st.Chain = p.Image, c.chain
		return edit(st, remakeSection, func(r *remakeInVM) error { r.Done = true; return nil })
	})
	if err != nil {
		return err
	}
	return e.dropOld(ctx, t, p)
}

// dropOld ends a remake of t's VM whose new machine is done (p.Done):
// it deletes the old machine, set aside, then ends the remake (clear).
func (e remaking) dropOld(ctx context.Context, t *found, p remakeInVM) error {
	vm := t.f.Name
	aside, _ := vmname.Aside(vm)
	if err := t.b.Delete(ctx, aside); err != nil && !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	if err := e.clear(ctx, t); err != nil {
		return err
	}
	e.Info(msg.Remade(p.Image, vm))
	return nil
}

// undo is a remake's cleanup, after a failure or an interrupt, until
// committed: it deletes the new machine and puts the old one back, as it
// was (putBack), and ends the remake. When it can't, the remake stays
// recorded, for the next devvm to finish.
func (e remaking) undo(t *found, committed *atomic.Bool) func(context.Context) {
	return func(ctx context.Context) {
		if committed.Load() {
			return
		}
		vm := t.f.Name
		e.Info(msg.RemakeUndone(vm))
		if err := e.putBack(ctx, t); err != nil {
			e.Warn(msg.RemakeUndoFailed(t.f.Dir, vm))
			return
		}
		if err := e.clear(ctx, t); err != nil {
			e.Warn(msg.RemakeUndoFailed(t.f.Dir, vm))
		}
	}
}

// putBack deletes t's VM's new machine, if it was made, and gives its old
// one, set aside, its name again: its next start makes it what its state
// asks for, as for any VM.
func (e remaking) putBack(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	aside, _ := vmname.Aside(vm)
	if _, err := b.Inspect(ctx, aside); err != nil {
		return err
	}
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	linkRuns(e).stop(ctx, vm)
	if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	t.exists = false
	if err := b.Rename(ctx, aside, vm); err != nil {
		return err
	}
	t.exists = true
	// The machine by its name is the old one again.
	return update(ctx, t.st, remakeSection, func(r *remakeInVM) error { r.Made = false; return nil })
}

// clear ends t's VM's remake: its section goes, then its saved home
// folder, which stays while the section does.
func (e remaking) clear(ctx context.Context, t *found) error {
	if err := update(ctx, t.st, remakeSection, func(r *remakeInVM) error { *r = remakeInVM{}; return nil }); err != nil {
		return err
	}
	if err := os.Remove(e.Dirs.VM(t.f.Name).Home()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// finishPending finishes a remake of t's VM that didn't (one a devvm
// that was killed left), asking nothing. While its old machine is set
// aside, or it was never set aside, the VM is put back as it was and made
// again from the start (remake). Once the old one is gone, the new one,
// made or not, never ran a shell: it's made again, with the home folder
// saved for it. Once the new one is done, only the old one is deleted
// (dropOld).
func (e remaking) finishPending(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	p := sectionOf[remakeInVM](t.st, remakeSection)
	e.Info(msg.RemakeResuming(p.Image, vm))
	if p.Done {
		return e.dropOld(ctx, t, p)
	}
	aside, _ := vmname.Aside(vm)
	_, err := b.Inspect(ctx, aside)
	again := err == nil || t.exists && !p.Made
	switch {
	case err != nil && !errors.Is(err, backend.ErrNotFound):
		return err
	case again:
		if err == nil {
			if err := e.putBack(ctx, t); err != nil {
				return err
			}
		}
		if err := e.clear(ctx, t); err != nil {
			return err
		}
		if err := e.check(t, p.Image); err != nil {
			return err
		}
		p.Made = false
		return e.remake(ctx, t, p)
	}
	return e.makeNew(ctx, t, p)
}

// makeNew finishes a remake of t's VM whose old machine is gone, for
// finishPending: checked as any remake is, it deletes the new machine,
// if it was made, and makes it again with the home folder saved for it.
func (e remaking) makeNew(ctx context.Context, t *found, p remakeInVM) error {
	vm, b := t.f.Name, t.b
	if err := e.check(t, p.Image); err != nil {
		return err
	}
	if t.exists {
		if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
			return err
		}
		t.exists = false
	}
	if _, err := os.Stat(e.Dirs.VM(vm).Home()); p.Home && err != nil {
		e.Warn(msg.RemakeHomeLost(vm))
		p.Home = false
	}
	c, err := e.build(ctx, t, p.Image)
	if err != nil {
		return err
	}
	return e.finish(ctx, t, p, c, new(atomic.Bool))
}
