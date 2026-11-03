package vm

import (
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/build"
	"github.com/w3cj/devvm/internal/devcontainer"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/settings"
)

// PickRow is one row of the tools picker: a toolchain, an agent or a
// layer, what it is or why it's checked, whether it's checked, and
// whether it's shown as it is (a VM's layers, which only a new VM gets).
type PickRow struct {
	Name, Desc string
	On, Fixed  bool
}

// PickPart is a heading of the tools picker and its rows.
type PickPart struct {
	Heading msg.M
	Rows    []PickRow
}

// Parts are what a new VM gets besides base: its toolchains, its agents
// and its layers, each a part of the picker and a flag of its own.
type Parts struct {
	Toolchains, Agents, Layers []string
}

// tools is the parts' shared tools, in a VM's order.
func (p Parts) tools() []string { return image.SortTools(slices.Concat(p.Toolchains, p.Agents)) }

// image is the image the parts' layers make.
func (p Parts) image() string { return image.Of(p.Layers) }

// listOrNone is names as a message shows a list: space-separated, or
// none.
func listOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, " ")
}

// partsDefault is what a new VM gets of each part when its flag is left
// out, and why: the toolchains the folder's files call for and those its
// devcontainer.json's features map to, the agents devvm setup chose, and
// the layers the devcontainer.json's features map to. why names the file
// that calls for each toolchain or layer.
type partsDefault struct {
	Parts
	why   map[string]string
	setup []string
}

// defaults works out a new VM's default parts for the folder dir with
// its devcontainer.json's plan dc (nil for none), and the agents def
// chooses. A layer a project file alone calls for, such as nix for a
// flake.nix, isn't one: only its reason is kept, for the picker.
func (e creation) defaults(dir string, dc *devcontainer.Plan, def settings.Defaults) partsDefault {
	d := partsDefault{why: map[string]string{}}
	for _, f := range project.DetectFiles(dir) {
		d.why[f.Name] = f.File
		if image.IsTool(f.Name) {
			d.Toolchains = append(d.Toolchains, f.Name)
		}
	}
	if dc != nil {
		for _, p := range dc.Layers {
			if _, ok := d.why[p]; !ok {
				d.why[p] = dc.File
			}
			if image.IsTool(p) {
				d.Toolchains = append(d.Toolchains, p)
			} else {
				d.Layers = append(d.Layers, p)
			}
		}
	}
	d.Toolchains = image.SortTools(d.Toolchains)
	d.Agents = image.AgentsOf(def.Agents.Names)
	d.setup = d.Agents
	return d
}

// chooseParts chooses what a new VM gets of each part: the flags', each
// the exact list for its part, and the defaults for those left out (def's
// agents). In a terminal without any of the flags, the picker shows the
// defaults checked; otherwise an info line says what the VM gets. The
// layers are checked against b's.
func (e creation) chooseParts(o Options, vm, dir string, dc *devcontainer.Plan, b *build.Env, def settings.Defaults) (Parts, error) {
	d := e.defaults(dir, dc, def)
	p := d.Parts
	given := false
	for _, f := range []struct {
		flag *[]string
		part *[]string
	}{{o.Toolchains, &p.Toolchains}, {o.Agents, &p.Agents}, {o.Layers, &p.Layers}} {
		if f.flag != nil {
			*f.part, given = *f.flag, true
		}
	}
	if !given && e.CanPick() && e.PickParts != nil {
		p = e.pickNew(vm, d, b)
	}
	p.Layers = image.LayersOf(p.image())
	if err := b.CheckLayers(p.image()); err != nil {
		return Parts{}, err
	}
	if given || !e.CanPick() || e.PickParts == nil {
		e.Info(msg.NewVMGets(listOrNone(p.Layers), listOrNone(p.tools()), vm))
	}
	return p, nil
}

// pickNew shows the picker for a new VM, its defaults checked, and
// returns the parts chosen.
func (e creation) pickNew(vm string, d partsDefault, b *build.Env) Parts {
	why := func(name string) string {
		if f, ok := d.why[name]; ok {
			return msg.PickCalledFor(f).Line()
		}
		return ""
	}
	var toolchains, agents, layers []PickRow
	for _, t := range image.Toolchains {
		toolchains = append(toolchains, PickRow{Name: t, Desc: why(t), On: slices.Contains(d.Toolchains, t)})
	}
	for _, a := range image.Agents {
		r := PickRow{Name: a, On: slices.Contains(d.Agents, a)}
		if slices.Contains(d.setup, a) {
			r.Desc = msg.PickSetupAgent().Line()
		}
		agents = append(agents, r)
	}
	for _, l := range b.Layers()[1:] {
		desc := why(l)
		if desc == "" {
			desc = layerDesc(l)
		}
		layers = append(layers, PickRow{Name: l, Desc: desc, On: slices.Contains(d.Layers, l)})
	}
	got := e.PickParts(msg.PickNewHelp(vm), []PickPart{
		{msg.PickToolchains(), toolchains}, {msg.PickAgents(), agents}, {msg.PickLayers(), layers},
	})
	return Parts{Toolchains: checked(got[0]), Agents: checked(got[1]), Layers: checked(got[2])}
}

// layerDesc is what a layer is, as the picker says it.
func layerDesc(layer string) string {
	switch layer {
	case image.DockerLayer:
		return msg.PickLayerDocker().Line()
	case image.NixLayer:
		return msg.PickLayerNix().Line()
	}
	return msg.PickLayerYours().Line()
}

// checked is the names of a part's checked rows, in order.
func checked(p PickPart) []string {
	var out []string
	for _, r := range p.Rows {
		if r.On {
			out = append(out, r.Name)
		}
	}
	return out
}

// AgentsChoice chooses the coding agents new VMs get by default, which
// the picker checks and a VM made without one gets: from a flag (Set), or
// asked for (Ask). Existing VMs keep their agents.
type AgentsChoice struct {
	Dirs paths.Dirs
	// Pick shows the question and returns the rows as chosen; Info says
	// what's saved.
	Pick func(msg.M, []PickPart) []PickPart
	Info func(msg.M)
}

// Set saves agents as new VMs' agents: none for none. A name that isn't an
// agent's is refused, and so is a choice config.toml makes
// (set_in_config).
func (c AgentsChoice) Set(agents []string) error {
	if err := CheckAgents(agents); err != nil {
		return err
	}
	sorted := append([]string{}, image.SortTools(agents)...)
	if _, err := settings.Save(c.Dirs, settings.Values{Agents: &sorted}); err != nil {
		return err
	}
	c.say(sorted)
	return nil
}

// Ask asks which agents new VMs get, starting from the ones in effect,
// and saves the answer; when config.toml chooses them, it says so and
// asks nothing.
func (c AgentsChoice) Ask() error {
	def, err := settings.Read(c.Dirs)
	if err != nil {
		return err
	}
	if def.Agents.Source == settings.FromConfig {
		c.Info(msg.SetupInConfig(c.Dirs.ConfigFile(), settings.KeyAgents))
		return nil
	}
	rows := make([]PickRow, len(image.Agents))
	for i, a := range image.Agents {
		rows[i] = PickRow{Name: a, On: slices.Contains(def.Agents.Names, a)}
	}
	got := c.Pick(msg.AgentsHelp(), []PickPart{{Rows: rows}})
	return c.Set(checked(got[0]))
}

func (c AgentsChoice) say(agents []string) {
	if len(agents) == 0 {
		c.Info(msg.DefaultAgentsNone())
		return
	}
	c.Info(msg.DefaultAgents(strings.Join(agents, " ")))
}

// CheckAgents refuses a name that isn't an agent's, as --agents does.
func CheckAgents(names []string) error {
	return CheckPart("--agents", names, image.Agents)
}

// CheckPart refuses a name of a part's flag (--tools, --agents, --layers)
// that isn't one of names, suggesting the closest (msg.Closest).
func CheckPart(flag string, given, names []string) error {
	for _, g := range given {
		if slices.Contains(names, g) {
			continue
		}
		if like := msg.Closest(g, names); like != "" {
			return msg.UnknownPartLike(flag, g, like)
		}
		return msg.UnknownPart(flag, g, strings.Join(names, " "))
	}
	return nil
}

// firstRunAgents asks which agents new VMs get on devvm's first run in a
// terminal, until they're chosen (def), unless --agents chose this VM's.
func (e creation) firstRunAgents(o Options, def settings.Defaults) error {
	if o.Agents != nil || !e.CanPick() || e.PickParts == nil || def.Agents.Chosen {
		return nil
	}
	return AgentsChoice{Dirs: e.Dirs, Pick: e.PickParts, Info: e.Info}.Ask()
}
