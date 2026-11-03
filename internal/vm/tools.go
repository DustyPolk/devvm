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
	t, err := e.existing(ctx, "")
	if err != nil {
		return nil, err
	}
	return tools.VMs{Env: e.Env.Tools}.Show(strings.Fields(t.st.Get(KeyTools)), toolTarget(t.b, t.st))
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
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	if err := e.change(ctx, t, add, gone); err != nil {
		return err
	}
	return Launch(e).open(ctx, o, t)
}

// change records Change's changes in the VM's state, building what it
// adds first.
func (e Tools) change(ctx context.Context, t *found, add, gone []string) error {
	vm, st := t.f.Name, t.st
	if len(gone) > 0 {
		names, files, err := tools.Remove(vm, st.Get(state.KeyImage), st.ToolNames(), strings.Fields(st.Get(KeyTools)), gone)
		if err != nil {
			return err
		}
		if err := st.Set(KeyTools, strings.Join(files, " ")); err != nil {
			return err
		}
		if err := st.Set(state.KeyToolNames, strings.Join(names, " ")); err != nil {
			return err
		}
	}
	have := st.ToolNames()
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
	if len(added) > 0 {
		if err := e.add(ctx, t, added); err != nil {
			return err
		}
	}
	if len(added) > 0 || len(gone) > 0 {
		e.Info(msg.VMToolsNow(listOrNone(st.ToolNames()), vm))
	}
	return nil
}

// add builds what the share lacks of the tools named, then records them
// in the VM's state: its shared tools and their files. It holds the lock
// VMs being made hold until the state lists them, so gc keeps them.
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
	if err := st.Set(KeyTools, strings.Join(append(strings.Fields(st.Get(KeyTools)), files...), " ")); err != nil {
		return err
	}
	return st.Set(state.KeyToolNames, strings.Join(image.SortTools(slices.Concat(st.ToolNames(), names)), " "))
}

// Pick shows the picker for the folder's VM, its toolchains and agents
// checked and its layers shown as they are, then makes what changed
// (Change). When nothing changed it says so and opens nothing.
func (e Tools) Pick(ctx context.Context, o Options) error {
	t, err := e.existing(ctx, o.Backend)
	if err != nil {
		return err
	}
	vm, st := t.f.Name, t.st
	have := st.ToolNames()
	row := func(name string) PickRow { return PickRow{Name: name, On: slices.Contains(have, name)} }
	var toolchains, agents, layers []PickRow
	for _, n := range image.Toolchains {
		toolchains = append(toolchains, row(n))
	}
	for _, n := range image.Agents {
		agents = append(agents, row(n))
	}
	b := e.builder(t.b, st.Get(state.KeyFrom))
	for _, l := range layerRows(b, st.Get(state.KeyImage)) {
		layers = append(layers, PickRow{Name: l, Desc: layerDesc(l), On: image.HasLayer(st.Get(state.KeyImage), l), Fixed: true})
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
func layerRows(b *build.Env, img string) []string {
	rows := b.Layers()[1:]
	for _, l := range image.LayersOf(img) {
		if !slices.Contains(rows, l) {
			rows = append(rows, l)
		}
	}
	return rows
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
	files, changed, err := tools.VMs{Env: e.Env.Tools}.Update(ctx, buildFn, vm, st.ToolNames(), strings.Fields(st.Get(KeyTools)), tg)
	if err == nil && changed {
		err = st.Set(KeyTools, strings.Join(files, " "))
	}
	unmaking()
	if err != nil {
		return err
	}
	return Launch(e).open(ctx, o, t)
}
