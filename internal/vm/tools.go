package vm

import (
	"context"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/build"
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
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return nil, err
	}
	tg, err := e.toolTarget(t.b, t.st)
	if err != nil {
		return nil, err
	}
	return tools.VMs{Env: e.Env.Tools}.Show(tools.Files(t.st), tg)
}

// Add adds shared tools (node, claude) to the folder's VM, then opens it
// (Change).
func (e Tools) Add(ctx context.Context, o Options, names []string) error {
	return e.Change(ctx, o, names, nil)
}

// Remove takes shared tools out of the folder's VM, then opens it
// (Change).
func (e Tools) Remove(ctx context.Context, o Options, names []string) error {
	return e.Change(ctx, o, nil, names)
}

// Change takes the shared tools gone out of the folder's VM and adds those
// in add, then opens it. Those it adds that the share lacks are built
// first, and a new agent's config folders are mounted when the launch
// finds them missing (UpdateAgentDirs), which asks before it restarts a
// running VM. Those it has already are said and left as they are. Those
// taken out aren't mounted from its next launch (an agent's folders
// neither), and what the VM installed itself stays.
func (e Tools) Change(ctx context.Context, o Options, add, gone []string) error {
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	if err := e.change(ctx, t, add, gone); err != nil {
		return err
	}
	return Launch(e).open(ctx, o, t)
}

// change records Change's changes in the VM's state, at once, building
// what it adds first. It holds the lock VMs being made hold until the
// state lists what it adds, so gc keeps their images.
func (e Tools) change(ctx context.Context, t *found, add, gone []string) error {
	vm, st := t.f.Name, t.st
	have := tools.Names(st)
	if len(gone) > 0 {
		// Refused before anything is built.
		names, _, err := tools.Remove(vm, image.Name(st.Image), have, tools.Files(st), gone)
		if err != nil {
			return err
		}
		have = names
	}
	var added, had []string
	for _, n := range add {
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
	if len(added) == 0 && len(gone) == 0 {
		return nil
	}
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return err
	}
	defer unmaking()
	files, err := e.build(ctx, t, added)
	if err != nil {
		return err
	}
	err = st.Update(ctx, func(st *state.VM) error {
		in, err := tools.Of(st)
		if err != nil {
			return err
		}
		if len(gone) > 0 {
			if in.Names, in.Made.Files, err = tools.Remove(vm, image.Name(st.Image), in.Names, in.Made.Files, gone); err != nil {
				return err
			}
		}
		in.Names = image.SortTools(slices.Compact(slices.Sorted(slices.Values(slices.Concat(in.Names, added)))))
		for _, f := range files {
			if !slices.Contains(in.Made.Files, f) {
				in.Made.Files = append(in.Made.Files, f)
			}
		}
		return tools.Set(st, in)
	})
	if err != nil {
		return err
	}
	e.Info(msg.VMToolsNow(listOrNone(tools.Names(st)), vm))
	return nil
}

// build builds what the share lacks of the tools named, and returns their
// tool images' files.
func (e Tools) build(ctx context.Context, t *found, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	builder := e.Builder(t.b, t.st.From)
	tg, err := Launch(e).buildTarget(ctx, builder, t.b, t.st)
	if err != nil {
		return nil, err
	}
	files, err := tools.Versions{Env: e.Env.Tools}.Files(ctx, names, tg)
	if err != nil {
		return nil, err
	}
	return files, Launch(e).buildTools(ctx, builder, tg, files)
}

// Pick shows the picker for the folder's VM, its toolchains and agents
// checked and its layers shown as they are, then makes what changed
// (Change). When nothing changed it says so and opens nothing.
func (e Tools) Pick(ctx context.Context, o Options) error {
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	have := tools.Names(st)
	row := func(name string) PickRow { return PickRow{Name: name, On: slices.Contains(have, name)} }
	var toolchains, agents, layers []PickRow
	for _, n := range image.Toolchains {
		toolchains = append(toolchains, row(n))
	}
	for _, n := range image.Agents {
		agents = append(agents, row(n))
	}
	b, img := e.Builder(t.b, st.From), image.Name(st.Image)
	for _, l := range layerRows(b, img) {
		layers = append(layers, PickRow{Name: l, Desc: layerDesc(l), On: img.Has(l), Fixed: true})
	}
	got := e.PickParts(msg.PickVMHelp(vm), []PickPart{
		{msg.PickToolchains(), toolchains}, {msg.PickAgents(), agents}, {msg.PickLayers(), layers},
	})
	want := slices.Concat(checked(got[0]), checked(got[1]))
	var add, gone []string
	for _, n := range want {
		if !slices.Contains(have, n) {
			add = append(add, n)
		}
	}
	for _, n := range have {
		if !slices.Contains(want, n) {
			gone = append(gone, n)
		}
	}
	if len(add) == 0 && len(gone) == 0 {
		e.Info(msg.NothingChanged())
		return nil
	}
	if err := e.change(ctx, t, add, gone); err != nil {
		return err
	}
	return Launch(e).open(ctx, o, t)
}

// layerRows are the layers the picker shows for a VM made from img: every
// layer there is, and any of img's that's gone since.
func layerRows(b *build.Env, img image.Name) []string {
	rows := b.Layers()[1:]
	for _, l := range img.Layers() {
		if !slices.Contains(rows, l) {
			rows = append(rows, l)
		}
	}
	return rows
}

// Update checks for newer shared tools and moves the folder's VM to them,
// built first, then opens it.
func (e Tools) Update(ctx context.Context, o Options) error {
	t, err := finder(e).target(ctx, need{has: hasMade})
	if err != nil {
		return err
	}
	if _, err := e.update(ctx, t); err != nil {
		return err
	}
	return Launch(e).open(ctx, o, t)
}

// update checks for newer shared tools and records them in t's VM's
// state, built first (tools.VMs.Update), which they take effect from as
// devvm opens a shell in it: devvm tools update's, and devvm update's
// first step. changed is false when it has the newest already.
func (e Tools) update(ctx context.Context, t *found) (changed bool, err error) {
	vm, st := t.f.Name, t.st
	builder := e.Builder(t.b, st.From)
	tg, err := Launch(e).buildTarget(ctx, builder, t.b, st)
	if err != nil {
		return false, err
	}
	buildFn := func(ctx context.Context, files []string) error { return Launch(e).buildTools(ctx, builder, tg, files) }
	// Held until its state lists the new tools, as a new VM's.
	unmaking, err := tools.Making{Dirs: e.Dirs, Info: e.Info}.Hold(ctx)
	if err != nil {
		return false, err
	}
	files, changed, err := tools.VMs{Env: e.Env.Tools}.Update(ctx, buildFn, vm, tools.Names(st), tools.Files(st), tg)
	if err == nil && changed {
		err = st.Update(ctx, func(st *state.VM) error {
			in, err := tools.Of(st)
			if err != nil {
				return err
			}
			in.Made.Files = files
			return tools.Set(st, in)
		})
	}
	unmaking()
	return changed && err == nil, err
}
