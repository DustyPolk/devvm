package vm

import (
	"context"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// Tools shows the folder's VM's shared tools, adds them, takes them out
// and moves it to the newest. Each change is to its state alone, as its
// tools are mounted from the share: nothing in its disk changes, and the
// change takes effect as devvm opens a shell in it.
type Tools struct{ *Env }

// List returns the folder's VM's shared tools, each with the lock's
// version when it's another and whether the share has that one built.
// The lock is read as it is: List never goes online.
func (e Tools) List(ctx context.Context) ([]tools.VMTool, error) {
	t, err := e.existing(ctx, "")
	if err != nil {
		return nil, err
	}
	return tools.VMs{Env: e.Env.Tools}.Show(strings.Fields(t.st.Get(KeyTools)), toolTarget(t.b, t.st))
}

// Add adds shared tools (names as images name them: node, claude) to the
// folder's VM, then opens it: those the share lacks are built first, and
// they join its image name and its list. A new agent's config folders
// are mounted when the launch finds them missing (UpdateAgentDirs), which
// asks before it restarts a running VM. Those it has already are said
// and left as they are.
func (e Tools) Add(ctx context.Context, o Options, names []string) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	have := strings.Split(st.Get(state.KeyImage), "+")
	var added, had []string
	for _, n := range names {
		switch {
		case slices.Contains(have, n):
			had = append(had, n)
		case !slices.Contains(added, n):
			added = append(added, n)
		}
	}
	if len(had) > 0 {
		e.Info(msg.HasTools(strings.Join(had, " "), vm))
	}
	if len(added) > 0 {
		if err := e.add(ctx, t, added); err != nil {
			return err
		}
		e.Info(msg.VMIsNow(st.Get(state.KeyImage), vm))
	}
	o.Image = ""
	return Launch(e).open(ctx, o, t)
}

// add builds what the share lacks of the tools named, then records them
// in the VM's state: its image name and its list. It holds the lock VMs
// being made hold until the state lists them, so gc keeps them.
func (e Tools) add(ctx context.Context, t *found, names []string) error {
	st := t.st
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	defer unmaking()
	tg := toolTarget(t.b, st)
	files, err := tools.Versions{Env: e.Env.Tools}.Files(ctx, names, tg)
	if err != nil {
		return err
	}
	builder := e.builder(t.b, st.Get(state.KeyFrom))
	if err := Launch(e).buildTools(ctx, builder, tg, files); err != nil {
		return err
	}
	img := image.Normalize(st.Get(state.KeyImage) + "+" + strings.Join(names, "+"))
	if err := st.Set(KeyTools, strings.Join(append(strings.Fields(st.Get(KeyTools)), files...), " ")); err != nil {
		return err
	}
	return st.Set(state.KeyImage, img)
}

// Remove takes shared tools out of the folder's VM, then opens it: they
// leave its image name and its list, and aren't mounted from its next
// launch (an agent's folders neither). What the VM installed itself
// stays.
func (e Tools) Remove(ctx context.Context, o Options, names []string) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	img, files, err := tools.Remove(vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), names)
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
	return Launch(e).open(ctx, o, t)
}

// Update checks for newer shared tools and moves the folder's VM to them,
// built first, then opens it.
func (e Tools) Update(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	builder := e.builder(t.b, st.Get(state.KeyFrom))
	tg := toolTarget(t.b, st)
	buildFn := func(ctx context.Context, files []string) error { return Launch(e).buildTools(ctx, builder, tg, files) }
	// Held until its state lists the new tools, as a new VM's.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	files, changed, err := tools.VMs{Env: e.Env.Tools}.Update(ctx, buildFn, vm, st.Get(state.KeyImage), strings.Fields(st.Get(KeyTools)), tg)
	if err == nil && changed {
		err = st.Set(KeyTools, strings.Join(files, " "))
	}
	unmaking()
	if err != nil {
		return err
	}
	o.Image = ""
	return Launch(e).open(ctx, o, t)
}
