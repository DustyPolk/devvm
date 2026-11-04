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
	"github.com/w3cj/devvm/internal/sshagent"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
	"github.com/w3cj/devvm/internal/trust"
	"github.com/w3cj/devvm/internal/vmname"
)

// RemakeOptions are the flags of a command that makes the folder's VM
// again (devvm layer add).
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
	t, resumed, err := e.toRemake(ctx, command)
	if err != nil || t == nil {
		return err
	}
	vm, st := t.f.Name, t.st
	img, had := addedImage(st.Image, layers)
	if len(had) > 0 {
		e.Info(msg.HasLayers(strings.Join(had, " "), vm))
	}
	if img == st.Image {
		if resumed {
			return Launch(e).open(ctx, Options{}, t)
		}
		return nil
	}
	return e.remakeTo(ctx, t, img, command, o)
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

// toRemake finds the folder's VM for a command that makes it again
// (command, as typed), and finishes a remake of it that didn't
// (resumed). A parent folder's VM is made again only after asking in a
// terminal, and never without one; nil when the user declines. It
// refuses a VM that's missing, made for another folder, unfinished, or
// a --clone VM, whose clone is on its disk.
func (e Lifecycle) toRemake(ctx context.Context, command string) (t *found, resumed bool, err error) {
	if t, err = e.find(ctx); err != nil {
		return nil, false, err
	}
	vm := t.f.Name
	if t.sub != "" {
		if !e.Terminal() {
			return nil, false, msg.RemakeParentVM(command, t.f.Dir, vm)
		}
		if !e.Ask(msg.RemakeParentVMAsk(t.f.Dir, vm)) {
			return nil, false, nil
		}
	}
	if err := e.checkFolder(t.f, t.st); err != nil {
		return nil, false, err
	}
	if remakePending(t.st) {
		if err := remaking(e).finishPending(ctx, t); err != nil {
			return nil, false, err
		}
		resumed = true
	}
	switch {
	case !t.exists:
		return nil, false, msg.NoVM(t.f.Dir)
	case !t.st.Finished():
		return nil, false, msg.VMUnfinished(vm)
	case isClone(t.st):
		return nil, false, msg.RemakeClone(vm)
	}
	return t, resumed, nil
}

// remakeTo makes t's VM again from img (remaking.remake), once it's
// checked that it can, and the user agrees: asked in a terminal ([y/N]),
// which --force skips, and refused without either. Then it opens a shell
// in it.
func (e Lifecycle) remakeTo(ctx context.Context, t *found, img, command string, o RemakeOptions) error {
	vm := t.f.Name
	r := remaking(e)
	if err := r.check(t, img); err != nil {
		return err
	}
	if !o.Force {
		if !e.Terminal() {
			return msg.ForceNeeded(command)
		}
		ask := msg.RemakeConfirm(img, vm)
		if o.CleanHome {
			ask = msg.RemakeConfirmCleanHome(img, vm)
		}
		if !e.Ask(ask) {
			e.Info(msg.Kept(vm))
			return nil
		}
	}
	if err := r.remake(ctx, t, remakeInVM{Image: img, Command: command, Home: !o.CleanHome, NoExport: o.NoExport}); err != nil {
		return err
	}
	return Launch(e).open(ctx, Options{}, t)
}

// remaking makes a VM again from an image, keeping devvm's settings for
// it and its home folder: the one remake devvm layer add uses.
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
// home folder in, deletes its old machine (then committed is set) and
// records its new image, which ends its remake.
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
	aside, _ := vmname.Aside(vm)
	if err := b.Delete(ctx, aside); err != nil && !errors.Is(err, backend.ErrNotFound) {
		return err
	}
	committed.Store(true)
	err = st.Update(ctx, func(st *state.VM) error {
		st.Image, st.Chain = p.Image, c.chain
		return state.SetSection(st, remakeSection, remakeInVM{})
	})
	if err != nil {
		return err
	}
	if err := os.Remove(e.Dirs.VM(vm).Home()); err != nil && !errors.Is(err, fs.ErrNotExist) {
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
	return nil
}

// clear ends t's VM's remake: its section and its saved home folder go.
func (e remaking) clear(ctx context.Context, t *found) error {
	err := update(ctx, t.st, remakeSection, func(r *remakeInVM) error { *r = remakeInVM{}; return nil })
	if rerr := os.Remove(e.Dirs.VM(t.f.Name).Home()); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		err = errors.Join(err, rerr)
	}
	return err
}

// finishPending finishes a remake of t's VM that didn't (one a devvm
// that was killed left), asking nothing. While its old machine is set
// aside, or it was never set aside, the VM is put back as it was and made
// again from the start (remake). Once the old one is gone, the new one,
// made or not, never ran a shell: it's made again, with the home folder
// saved for it.
func (e remaking) finishPending(ctx context.Context, t *found) error {
	vm, b := t.f.Name, t.b
	p := sectionOf[remakeInVM](t.st, remakeSection)
	e.Info(msg.RemakeResuming(p.Image, vm))
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
