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
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ports"
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/vmname"
)

// RemakeOptions are the flags of a command that makes the folder's VM
// again (devvm layer add, devvm layer rm, devvm update, devvm clone).
type RemakeOptions struct {
	Force    bool // --force: no "make it again?" asked, which needs a terminal otherwise
	NoExport bool // --no-export: the agents' history isn't exported first
	// CleanHome (--clean-home): the new disk has the image's home folder,
	// not the VM's.
	CleanHome bool
	// NoCloneCheck (--no-clone-check, devvm clone off's): the clone isn't
	// checked for work the repo lacks first, which is lost.
	NoCloneCheck bool
}

// The clone modes a remake makes a VM in (remakeInVM.Clone), as devvm
// clone takes them: with its own clone of the repo, or mounting the
// folder.
const (
	CloneOn  = "on"
	CloneOff = "off"
)

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
	// Clone is the mode its new machine is made in, CloneOn or CloneOff
	// (devvm clone on, devvm clone off); "" keeps the VM's. Once Made,
	// the clone section says the new machine's mode (switchedTo).
	Clone string `json:"clone,omitempty"`
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
	return e.remakeLayers(ctx, command, o, func(t *target) (string, error) {
		img, had := addedImage(image.Name(t.st.Image), layers)
		if len(had) > 0 {
			e.Info(msg.HasLayers(strings.Join(had, " "), t.f.Name))
		}
		return img.String(), nil
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
	return e.remakeLayers(ctx, command, o, func(t *target) (string, error) {
		img, lacks := removedImage(image.Name(t.st.Image), layers)
		if err := CheckLayers(lacks, e.Builder(t.b, t.st.From).Layers()[1:]); err != nil {
			return "", err
		}
		if len(lacks) > 0 {
			e.Info(msg.LacksLayers(strings.Join(lacks, " "), t.f.Name))
		}
		return img.String(), nil
	})
}

// remakeLayers makes the folder's VM again from the image that layers
// works out from it (devvm layer add's, devvm layer rm's), after the user
// agrees (remakeTo). When that's the image it has, nothing changes, but
// a VM whose remake was just finished gets its shell.
func (e Lifecycle) remakeLayers(ctx context.Context, command string, o RemakeOptions, layers func(*target) (string, error)) error {
	t, resumed, release, err := e.toRemake(ctx, command, false)
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
			return e.launcher.open(ctx, Options{}, t)
		}
		return nil
	}
	return e.remakeTo(ctx, t, plan(img, command, o), o)
}

// plan is the remake of a VM from img that command (as typed) asks for
// with o.
func plan(img, command string, o RemakeOptions) remakeInVM {
	return remakeInVM{Image: img, Command: command, Home: !o.CleanHome, NoExport: o.NoExport}
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
	t, resumed, release, err := e.toRemake(ctx, command, false)
	if err != nil || t == nil {
		return err
	}
	defer release()
	vm, st := t.f.Name, t.st
	changed, err := (build.Builds{Env: e.Builder(t.b, st.From)}).Outdated(st.Chain)
	if err != nil {
		return err
	}
	p := plan(st.Image, command, o)
	if len(changed) > 0 {
		e.Info(msg.VMChanged(strings.Join(changed, ", "), vm))
		if err := e.canRemake(t, p, o); err != nil {
			return err
		}
	}
	newer, err := e.vmTools.update(ctx, t)
	if err != nil {
		return err
	}
	switch {
	case len(changed) > 0 && e.agreed(t, p, o):
		return e.remakeNow(ctx, t, p)
	case newer || resumed:
		// Its newer tools take effect as its shell opens, kept or not.
		return e.launcher.open(ctx, Options{}, t)
	case len(changed) == 0:
		e.Info(msg.VMCurrent(st.Image, vm))
	}
	return nil
}

// addedImage is the image img makes with layers added on top, in the
// order given (inOrder), and those of layers img has already.
func addedImage(img image.Name, layers []string) (image.Name, []string) {
	was := img.Layers()
	var had []string
	for _, l := range layers {
		if slices.Contains(was, l) && !slices.Contains(had, l) {
			had = append(had, l)
		}
	}
	return image.NameOf(inOrder(was, slices.Concat(was, layers))), had
}

// removedImage is the image img makes with layers taken out, the rest in
// their order (base when none is left), and those of layers img lacks.
func removedImage(img image.Name, layers []string) (image.Name, []string) {
	was := img.Layers()
	var lacks []string
	for _, l := range layers {
		if !slices.Contains(was, l) && !slices.Contains(lacks, l) {
			lacks = append(lacks, l)
		}
	}
	return image.NameOf(slices.DeleteFunc(was, func(l string) bool { return slices.Contains(layers, l) })), lacks
}

// toRemake finds the folder's VM for a command that makes it again
// (command, as typed), and finishes a remake of it that didn't
// (resumed). A parent folder's VM is made again only after asking in a
// terminal, and never without one; nil when the user declines. It
// refuses a VM that's missing, made for another folder, unfinished, or,
// unless cloneOK (devvm clone's), a --clone VM, whose clone is on its
// disk. Once it has the VM, it holds the VM's remake lock (hold) until
// release.
func (e Lifecycle) toRemake(ctx context.Context, command string, cloneOK bool) (t *target, resumed bool, release func(), err error) {
	if t, err = e.finder.target(ctx, need{folder: true}); err != nil {
		return nil, false, nil, err
	}
	vm := t.f.Name
	if t.sub != "" {
		if !e.StdinIsTerminal() {
			return nil, false, nil, msg.RemakeParentVM(command, t.f.Dir, vm)
		}
		if !e.Ask(msg.RemakeParentVMAsk(t.f.Dir, vm)) {
			return nil, false, nil, nil
		}
	}
	if release, err = e.remaking.hold(ctx, t); err != nil {
		return nil, false, nil, err
	}
	if resumed, err = e.refuseRemake(ctx, t, cloneOK); err != nil {
		release()
		return nil, false, nil, err
	}
	return t, resumed, release, nil
}

// refuseRemake finishes a remake of t's VM that didn't (resumed), then
// refuses a VM that's missing, unfinished, or, unless cloneOK, a --clone
// VM.
func (e Lifecycle) refuseRemake(ctx context.Context, t *target, cloneOK bool) (resumed bool, err error) {
	vm := t.f.Name
	if remakePending(t.st) {
		if err := e.remaking.finishPending(ctx, t); err != nil {
			return false, err
		}
		resumed = true
	}
	if !t.exists {
		return false, msg.NoVM(t.f.Dir)
	}
	if err := refuseUnfinished(vm, t.st); err != nil {
		return false, err
	}
	if isClone(t.st) && !cloneOK {
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
func (e remaking) hold(ctx context.Context, t *target) (release func(), err error) {
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
		if err := t.refresh(ctx); err != nil {
			unlock()
			return nil, err
		}
	}
	return unlock, nil
}

// remakeTo makes t's VM again as p says (remaking.remake), once it's
// checked that it can, and the user agrees: asked in a terminal ([y/N]),
// which --force skips, and refused without either. Then it opens a shell
// in it.
func (e Lifecycle) remakeTo(ctx context.Context, t *target, p remakeInVM, o RemakeOptions) error {
	if err := e.canRemake(t, p, o); err != nil {
		return err
	}
	if !e.agreed(t, p, o) {
		return nil
	}
	return e.remakeNow(ctx, t, p)
}

// agreed reports whether the user agrees to make t's VM again from p's
// image, once canRemake has checked that it can: asked in a terminal
// ([y/N]), which --force skips. A no is said (kept).
func (e Lifecycle) agreed(t *target, p remakeInVM, o RemakeOptions) bool {
	vm := t.f.Name
	if o.Force {
		return true
	}
	ask := msg.RemakeConfirm(p.Image, vm)
	if !p.Home {
		ask = msg.RemakeConfirmCleanHome(p.Image, vm)
	}
	if !e.Ask(ask) {
		e.Info(msg.Kept(vm))
		return false
	}
	return true
}

// canRemake refuses to make t's VM again as p says when remaking.check
// does, or when there's neither --force nor a terminal to ask in.
func (e Lifecycle) canRemake(t *target, p remakeInVM, o RemakeOptions) error {
	if err := e.remaking.check(t, p); err != nil {
		return err
	}
	if !o.Force && !e.StdinIsTerminal() {
		return msg.ForceNeeded(p.Command)
	}
	return nil
}

// remakeNow makes t's VM again as p says (remaking.remake), then opens a
// shell in it.
func (e Lifecycle) remakeNow(ctx context.Context, t *target, p remakeInVM) error {
	if err := e.remaking.remake(ctx, t, p); err != nil {
		return err
	}
	return e.launcher.open(ctx, Options{}, t)
}

// remaking makes a VM again from an image, keeping devvm's settings for
// it and its home folder: the one remake devvm layer add, devvm layer rm,
// devvm update and devvm clone use.
type remaking struct{ *Env }

// check refuses, before anything runs, to make t's VM again as p says
// when one of its image's layers is gone, or its backend can't run what
// the VM needs with it (requirements: its ports, and a --clone VM's ssh
// too), or the disk lacks room for a VM.
func (e remaking) check(t *target, p remakeInVM) error {
	vm, b, img := t.f.Name, t.b, image.Name(p.Image)
	if err := (build.Names{Env: e.Builder(b, t.st.From)}).Check(img); err != nil {
		return err
	}
	spec := e.spec(t, p)
	cur, err := ports.Of(t.st)
	if err != nil {
		return err
	}
	forward, err := ports.Forwarding(cur.Forward)
	if err != nil {
		return err
	}
	// Its clone is copied in and fetched over ssh, as up's.
	need := needs{spec: &spec, net: confinementFor(t.st).Net, ports: forward, img: img, ssh: spec.CloneAt != ""}
	if err := e.Registry.Require(b, vm, requirements(need)...); err != nil {
		return err
	}
	return (backend.Space{Free: e.FreeSpace}).Check(b, vm, func(f backend.Floors) uint64 { return f.VM })
}

// spec is the new machine of t's VM as its state asks for it
// (creation.machineSpec, as up makes a new VM's): its shared tools, SSH
// agent and size, in the clone mode p makes it in (wantsClone).
func (e remaking) spec(t *target, p remakeInVM) backend.VMSpec {
	st := t.st
	return e.creation.machineSpec(t.b, t.f, tools.Names(st), sshagent.Forwards(sshagent.Mode(st)),
		sectionOf[Size](st, sizeSection), wantsClone(st, p))
}

// wantsClone reports whether p makes st's VM's new machine with its own
// clone of the repo: as p.Clone says, else as the VM is.
func wantsClone(st *state.VM, p remakeInVM) bool {
	switch p.Clone {
	case CloneOn:
		return true
	case CloneOff:
		return false
	}
	return isClone(st)
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
func (e remaking) remake(ctx context.Context, t *target, p remakeInVM) error {
	vm := t.f.Name
	chain, err := e.build(ctx, t, p.Image)
	if err != nil {
		return err
	}
	if !p.NoExport && len(image.AgentsOf(tools.Names(t.st))) > 0 {
		if err := e.lifecycle.export(ctx, t, "", msg.RemakeExportFailed(p.Command, vm)); err != nil {
			return err
		}
	}
	if p.Home {
		if err := t.ensureRunning(ctx); err != nil {
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

// build builds img (a remake's, as its record has it) for t's VM, as a
// new VM's is built, and returns its build chain.
func (e remaking) build(ctx context.Context, t *target, recorded string) (chainOf, error) {
	b, img := e.Builder(t.b, t.st.From), image.Name(recorded)
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
func (e remaking) setAside(ctx context.Context, t *target) error {
	vm := t.f.Name
	if err := t.stop(ctx); err != nil {
		return err
	}
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	e.linkRuns.stop(ctx, vm)
	aside, _ := vmname.Aside(vm)
	return t.b.Rename(ctx, vm, aside)
}

// finish makes t's VM's new machine from p's image (built: chain) with
// what its state records, in p's clone mode, starts it as a launch does,
// puts its saved home folder in (then committed is set), records its new
// image and deletes its old machine, which ends its remake (dropOld).
func (e remaking) finish(ctx context.Context, t *target, p remakeInVM, c chainOf, committed *atomic.Bool) error {
	vm, st, b := t.f.Name, t.st, t.b
	e.Info(msg.CreatingFrom(p.Image, vm))
	e.LogNote()
	if err := e.creation.makeMachine(ctx, b, st, c.key, e.spec(t, p)); err != nil {
		return err
	}
	t.exists = true
	// The new machine is the one by its name now, in its clone mode, and
	// its devcontainer.json's commands run again on its new disk.
	mode := ""
	if p.Clone == CloneOff {
		mode = e.Protector(t.f.Dir).NewMode(false)
	}
	err := st.Update(ctx, func(st *state.VM) error {
		if err := edit(st, remakeSection, func(r *remakeInVM) error { r.Made = true; return nil }); err != nil {
			return err
		}
		if err := switchedTo(st, p.Clone, mode); err != nil {
			return err
		}
		return edit(st, devcontainerSection, func(d *devcontainerInVM) error { d.Made = devcontainerMade{}; return nil })
	})
	if err != nil {
		return err
	}
	if p.Clone == CloneOff {
		if err := e.mountFolder(ctx, t); err != nil {
			return err
		}
	}
	if _, err := e.starter.bringUp(ctx, t, upOpts{}); err != nil {
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
// it deletes the old machine, set aside, then what the old one's clone
// mode left on the host (leftBehind), then ends the remake (clear).
func (e remaking) dropOld(ctx context.Context, t *target, p remakeInVM) error {
	vm := t.f.Name
	aside, _ := vmname.Aside(vm)
	if err := t.b.Delete(ctx, aside); err != nil && !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	if err := e.leftBehind(ctx, t, p.Clone); err != nil {
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
func (e remaking) undo(t *target, committed *atomic.Bool) func(context.Context) {
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
func (e remaking) putBack(ctx context.Context, t *target) error {
	vm, b := t.f.Name, t.b
	aside, _ := vmname.Aside(vm)
	if _, err := b.Inspect(ctx, aside); err != nil {
		return err
	}
	e.Proxy.Stop(vm)
	e.Forwarders.Stop(vm)
	e.linkRuns.stop(ctx, vm)
	if err := b.Delete(ctx, vm); err != nil && !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	t.exists = false
	if err := b.Rename(ctx, aside, vm); err != nil {
		return err
	}
	t.exists = true
	// The machine by its name is the old one again, in its clone mode.
	r := sectionOf[remakeInVM](t.st, remakeSection)
	if r.Made && r.Clone == CloneOff {
		e.unmountFolder(ctx, t)
	}
	return t.st.Update(ctx, func(st *state.VM) error {
		if r.Made {
			if err := switchedBack(st, r.Clone); err != nil {
				return err
			}
		}
		return edit(st, remakeSection, func(r *remakeInVM) error { r.Made = false; return nil })
	})
}

// clear ends t's VM's remake: its section goes, then its saved home
// folder, which stays while the section does.
func (e remaking) clear(ctx context.Context, t *target) error {
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
func (e remaking) finishPending(ctx context.Context, t *target) error {
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
		if err := e.check(t, p); err != nil {
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
func (e remaking) makeNew(ctx context.Context, t *target, p remakeInVM) error {
	vm, b := t.f.Name, t.b
	if err := e.check(t, p); err != nil {
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
