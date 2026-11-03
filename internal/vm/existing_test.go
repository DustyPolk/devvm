package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
)

// With network policy: a running VM isn't started again and keeps its
// proxy; the self-check runs again on every launch. A stopped one is
// started confined.
func TestOpenNetworkPolicy(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	l := Launch{e}
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	st := existingVM(t, e, Options{NetworkPolicy: true})
	vm := st.Name()
	pid, ok := e.Proxy.PID(vm)
	if !ok {
		t.Fatal("no proxy")
	}
	before := len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	ms := f.Methods()[before:]
	if slices.Contains(ms, "Start") || !slices.Contains(ms, "Confined") {
		t.Errorf("calls = %q", ms)
	}
	if again, _ := e.Proxy.PID(vm); again != pid {
		t.Errorf("proxy %d, was %d", again, pid)
	}
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before = len(f.Calls())
	if err := l.Up(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if c, _ := call(f, "Start").Args[1].(backend.Confinement); !slices.Contains(f.Methods()[before:], "Start") || c.Net != backend.NetProxy {
		t.Errorf("calls = %q, confinement %+v", f.Methods()[before:], c)
	}
	// Started outside devvm: no terminal to ask in, so it's stopped.
	f.Unconfined = true
	if err := l.Up(t.Context(), Options{}); msg.NamesOf(err) != "net_unenforced_stop vm="+vm {
		t.Errorf("Up unconfined = %v", msg.NamesOf(err))
	}
	if f.Instances[vm].Status != backend.StatusStopped || len(*execs) != 3 {
		t.Errorf("status %s, execs %d", f.Instances[vm].Status, len(*execs))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(netpolicy.KeyCheck) != netpolicy.CheckUnenforced {
		t.Errorf("net_check = %q", st.Get(netpolicy.KeyCheck))
	}
}

// --tools, --agents and --layers change nothing on a VM that exists: it
// opens as it is, with a warning naming devvm tools.
func TestPartsFlagsIgnored(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	st := existingVM(t, e, Options{Toolchains: &[]string{"go"}, NoNetworkPolicy: true})
	vm := st.Name()
	for _, o := range []Options{{Agents: &[]string{"claude"}}, {Toolchains: &[]string{}}, {Layers: &[]string{"docker"}}} {
		var warned msg.Recorder
		e.Warn = warned.Add
		before := len(f.Calls())
		if err := l.Up(t.Context(), o); err != nil {
			t.Fatalf("Up %+v = %v", o, msg.NamesOf(err))
		}
		if !slices.Equal(warned.Names(), []string{"parts_flags_ignored vm=" + vm}) {
			t.Errorf("Up %+v warned %q", o, warned.Names())
		}
		if ms := f.Methods()[before:]; slices.Contains(ms, "BuildTools") || slices.Contains(ms, "BuildImage") {
			t.Errorf("Up %+v calls %q", o, ms)
		}
	}
	if st, _ = state.ReadVM(e.Dirs, vm); st.Get(state.KeyImage) != "base" || st.Get(state.KeyToolNames) != "go" {
		t.Errorf("image %s, tool_names %s", st.Get(state.KeyImage), st.Get(state.KeyToolNames))
	}
}

// devvm tools add: the tools the share lacks built, then in the VM's
// list; a new agent's config folders mounted at once on a stopped VM,
// after asking on a running one; those it has said and left.
func TestToolsAdd(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	tl := Tools{e}
	st := existingVM(t, e, Options{Toolchains: &[]string{"go"}, NoNetworkPolicy: true})
	vm := st.Name()
	skills := filepath.Join(e.Dirs.Home, ".claude", "skills")
	if err := os.MkdirAll(skills, 0o700); err != nil {
		t.Fatal(err)
	}
	var said msg.Recorder
	e.Info = said.Add
	if err := f.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	before := len(f.Calls())
	if err := tl.Add(t.Context(), Options{}, []string{"go", "claude", "claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyImage) != "base" || st.Get(state.KeyToolNames) != "go claude" || !strings.Contains(st.Get(KeyTools), " claude-") {
		t.Errorf("state image=%s tool_names=%s tools=%s", st.Get(state.KeyImage), st.Get(state.KeyToolNames), st.Get(KeyTools))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "SetAgentDirs"), slices.Index(ms, "Start"); !slices.Contains(ms, "BuildTools") || i < 0 || j < i {
		t.Errorf("calls = %q", ms)
	}
	if dirs := f.Instances[vm].AgentDirs; !slices.Equal(dirs, []string{skills}) {
		t.Errorf("agent dirs = %q", dirs)
	}
	if !slices.Contains(said.Names(), "has_tools tools=go vm="+vm) || !slices.Contains(said.Names(), "vm_tools_now tools=go claude vm="+vm) {
		t.Errorf("said %q", said.Names())
	}

	// Running: a new agent's folders after a restart it asks for.
	prompts := filepath.Join(e.Dirs.Home, ".codex", "prompts")
	if err := os.MkdirAll(prompts, 0o700); err != nil {
		t.Fatal(err)
	}
	asked := answers(t, e, true)
	before = len(f.Calls())
	if err := tl.Add(t.Context(), Options{}, []string{"codex"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); st.Get(state.KeyToolNames) != "go claude codex" {
		t.Errorf("tool_names = %s", st.Get(state.KeyToolNames))
	}
	ms = f.Methods()[before:]
	if !slices.Equal(*asked, []string{"update_mounts vm=" + vm}) || !slices.Contains(ms, "Stop") ||
		!slices.Contains(f.Instances[vm].AgentDirs, prompts) {
		t.Errorf("asked %q, calls %q, agent dirs %q", *asked, ms, f.Instances[vm].AgentDirs)
	}
}

// devvm tools in a terminal: the picker shows the VM's toolchains and
// agents checked and its layers fixed, as they are; what's checked and
// unchecked becomes devvm tools add and rm, then the VM opens. Nothing
// changed opens nothing.
func TestToolsPick(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	tl := Tools{e}
	st := existingVM(t, e, Options{Toolchains: &[]string{"go"}, Agents: &[]string{"claude"}, Layers: &[]string{"nix"}, NoNetworkPolicy: true})
	vm := st.Name()
	var shown []PickPart
	pick := func(check, uncheck string) {
		e.PickParts = func(help msg.M, parts []PickPart) []PickPart {
			if help.ID() != "pick_vm_help" {
				t.Errorf("asked %s", help.Names())
			}
			shown = parts
			out := make([]PickPart, len(parts))
			for i, p := range parts {
				out[i] = PickPart{Heading: p.Heading, Rows: slices.Clone(p.Rows)}
				for j, r := range out[i].Rows {
					if r.Name == check {
						out[i].Rows[j].On = true
					}
					if r.Name == uncheck {
						out[i].Rows[j].On = false
					}
				}
			}
			return out
		}
	}
	pick("", "")
	before := len(*execs)
	var said msg.Recorder
	e.Info = said.Add
	if err := tl.Pick(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if len(*execs) != before || !slices.Equal(said.Names(), []string{"nothing_changed"}) {
		t.Errorf("unchanged: execs %d, said %q", len(*execs)-before, said.Names())
	}
	for _, r := range shown[2].Rows {
		if !r.Fixed || r.On != (r.Name == "nix") {
			t.Errorf("layer row %+v", r)
		}
	}
	if !shown[0].Rows[3].On || shown[0].Rows[3].Name != "go" || !shown[1].Rows[0].On || shown[1].Rows[0].Fixed {
		t.Errorf("rows %+v", shown[:2])
	}
	pick("python", "claude")
	n := len(f.Calls())
	if err := tl.Pick(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); st.Get(state.KeyToolNames) != "python go" || strings.Contains(st.Get(KeyTools), "claude") {
		t.Errorf("tool_names=%s tools=%s", st.Get(state.KeyToolNames), st.Get(KeyTools))
	}
	if ms := f.Methods()[n:]; !slices.Contains(ms, "BuildTools") || len(*execs) != before+1 {
		t.Errorf("calls %q, execs %d", ms, len(*execs)-before)
	}
}

// A new VM's default agents: set, refused with a did-you-mean, and asked
// for from the saved ones.
func TestAgentsChoice(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	var said msg.Recorder
	c := AgentsChoice{Dirs: e.Dirs, Info: said.Add}
	if err := c.Set([]string{"codex", "claude"}); err != nil {
		t.Fatal(err)
	}
	if agents, _, _ := state.ReadAgents(e.Dirs); !slices.Equal(agents, []string{"claude", "codex"}) {
		t.Errorf("saved %q", agents)
	}
	for given, want := range map[string]string{
		"claud":  "unknown_part_like flag=--agents given=claud like=claude",
		"python": "unknown_part flag=--agents given=python names=claude codex opencode",
	} {
		if err := c.Set([]string{given}); msg.NamesOf(err) != want {
			t.Errorf("Set(%s) = %v", given, msg.NamesOf(err))
		}
	}
	c.Pick = func(help msg.M, parts []PickPart) []PickPart {
		if help.ID() != "agents_help" || !parts[0].Rows[0].On || !parts[0].Rows[1].On || parts[0].Rows[2].On {
			t.Errorf("asked %s with %+v", help.Names(), parts)
		}
		parts[0].Rows[0].On, parts[0].Rows[1].On = false, false
		return parts
	}
	if err := c.Ask(); err != nil {
		t.Fatal(err)
	}
	if agents, set, _ := state.ReadAgents(e.Dirs); !set || len(agents) != 0 {
		t.Errorf("saved %q, %v", agents, set)
	}
	if !slices.Equal(said.Names(), []string{"default_agents agents=claude codex", "default_agents_none"}) {
		t.Errorf("said %q", said.Names())
	}
}

// tools rm and update open the VM after changing its tools; restart stops
// it first.
func TestRemoveUpdateRestart(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	l := Launch{e}
	tl := Tools{e}
	st := existingVM(t, e, Options{Toolchains: &[]string{"go"}, Agents: &[]string{"claude"}, NoNetworkPolicy: true})
	vm := st.Name()
	if err := tl.Remove(t.Context(), Options{}, []string{"claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	if st.Get(state.KeyToolNames) != "go" || strings.Contains(st.Get(KeyTools), "claude") {
		t.Errorf("state tool_names=%s tools=%s", st.Get(state.KeyToolNames), st.Get(KeyTools))
	}
	if err := tl.Remove(t.Context(), Options{}, []string{"python"}); msg.NamesOf(err) != "vm_lacks has=go tool=python vm="+vm {
		t.Errorf("Remove python = %v", msg.NamesOf(err))
	}
	if err := tl.Update(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if ts, err := tl.List(t.Context()); err != nil || len(ts) != 1 || ts[0].Tool != "go" || ts[0].Newer != "" {
		t.Errorf("List = %+v, %v", ts, err)
	}
	// go added again after claude, so its file is listed after claude's:
	// still the newest.
	if err := tl.Add(t.Context(), Options{}, []string{"claude"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := tl.Remove(t.Context(), Options{}, []string{"go"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if err := tl.Add(t.Context(), Options{}, []string{"go"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	st, _ = state.ReadVM(e.Dirs, vm)
	had := st.Get(KeyTools)
	if err := tl.Update(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); !strings.HasPrefix(had, "claude-") || st.Get(KeyTools) != had {
		t.Errorf("Update after adding: tools %q, were %q", st.Get(KeyTools), had)
	}
	before := len(f.Calls())
	if err := l.Restart(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	ms := f.Methods()[before:]
	if i, j := slices.Index(ms, "Stop"), slices.Index(ms, "Start"); i < 0 || j < i {
		t.Errorf("restart calls = %q", ms)
	}
}
