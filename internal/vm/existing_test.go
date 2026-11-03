package vm

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/sshagent"
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
	if get(st, keyCheck) != netpolicy.CheckUnenforced {
		t.Errorf("net_check = %q", get(st, keyCheck))
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
	if st, _ = state.ReadVM(e.Dirs, vm); st.Image != "base" || get(st, keyToolNames) != "go" {
		t.Errorf("image %s, tool_names %s", st.Image, get(st, keyToolNames))
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
	if st.Image != "base" || get(st, keyToolNames) != "go claude" || !strings.Contains(get(st, KeyTools), " claude-") {
		t.Errorf("state image=%s tool_names=%s tools=%s", st.Image, get(st, keyToolNames), get(st, KeyTools))
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
	if st, _ = state.ReadVM(e.Dirs, vm); get(st, keyToolNames) != "go claude codex" {
		t.Errorf("tool_names = %s", get(st, keyToolNames))
	}
	ms = f.Methods()[before:]
	if !slices.Equal(*asked, []string{"restart_now vm=" + vm}) || !slices.Contains(ms, "Stop") ||
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
	if st, _ = state.ReadVM(e.Dirs, vm); get(st, keyToolNames) != "python go" || strings.Contains(get(st, KeyTools), "claude") {
		t.Errorf("tool_names=%s tools=%s", get(st, keyToolNames), get(st, KeyTools))
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
	if def, _ := settings.Read(e.Dirs); !slices.Equal(def.Agents.Names, []string{"claude", "codex"}) {
		t.Errorf("saved %q", def.Agents.Names)
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
	if def, _ := settings.Read(e.Dirs); !def.Agents.Chosen || len(def.Agents.Names) != 0 {
		t.Errorf("saved %+v", def.Agents)
	}
	if !slices.Equal(said.Names(), []string{"default_agents agents=claude codex", "default_agents_none"}) {
		t.Errorf("said %q", said.Names())
	}
	// config.toml's agents: Set is refused, Ask says so and asks nothing.
	if err := os.MkdirAll(e.Dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.Dirs.ConfigFile(), []byte("agents = [\"codex\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Set([]string{"claude"}); msg.NamesOf(err) != "set_in_config file="+e.Dirs.ConfigFile()+" key=agents" {
		t.Errorf("Set with config.toml's: %v", msg.NamesOf(err))
	}
	c.Pick = func(msg.M, []PickPart) []PickPart { t.Error("asked"); return nil }
	if err := c.Ask(); err != nil {
		t.Fatal(err)
	}
	if got := said.Names()[len(said.Names())-1]; got != "setup_in_config file="+e.Dirs.ConfigFile()+" key=agents" {
		t.Errorf("said %q", got)
	}
}

// New VMs' default layers: set bottom first as given, refused with a
// did-you-mean, and asked for from the saved ones, each row saying what
// its layer is; config.toml's are refused and not asked for.
func TestLayersChoice(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	var said msg.Recorder
	c := LayersChoice{Dirs: e.Dirs, Layers: []string{"desktop", "docker", "mine", "nix"}, Info: said.Add}
	if err := c.Set([]string{"nix", "docker", "nix"}); err != nil {
		t.Fatal(err)
	}
	if def, _ := settings.Read(e.Dirs); !slices.Equal(def.Layers.Names, []string{"nix", "docker"}) {
		t.Errorf("saved %q", def.Layers.Names)
	}
	for given, want := range map[string]string{
		"dokcer": "unknown_part_like flag=--layers given=dokcer like=docker",
		"zzz":    "unknown_part flag=--layers given=zzz names=desktop docker mine nix",
	} {
		if err := c.Set([]string{given}); msg.NamesOf(err) != want {
			t.Errorf("Set(%s) = %v", given, msg.NamesOf(err))
		}
	}
	c.Pick = func(help msg.M, parts []PickPart) []PickPart {
		var on, descs []string
		for _, r := range parts[0].Rows {
			descs = append(descs, r.Desc)
			if r.On {
				on = append(on, r.Name)
			}
		}
		if help.ID() != "layers_help" || !slices.Equal(on, []string{"docker", "nix"}) ||
			!slices.Equal(descs, []string{"pick_layer_desktop", "pick_layer_docker", "pick_layer_yours", "pick_layer_nix"}) {
			t.Errorf("asked %s with %+v", help.Names(), parts)
		}
		parts[0].Rows[0].On, parts[0].Rows[1].On = true, false // desktop, not docker
		return parts
	}
	if err := c.Ask(); err != nil {
		t.Fatal(err)
	}
	if def, _ := settings.Read(e.Dirs); !slices.Equal(def.Layers.Names, []string{"nix", "desktop"}) {
		t.Errorf("saved %+v", def.Layers)
	}
	if err := c.Set(nil); err != nil {
		t.Fatal(err)
	}
	if def, _ := settings.Read(e.Dirs); !def.Layers.Chosen || len(def.Layers.Names) != 0 {
		t.Errorf("saved %+v", def.Layers)
	}
	if want := []string{"default_layers layers=nix docker", "default_layers layers=nix desktop", "default_layers_none"}; !slices.Equal(said.Names(), want) {
		t.Errorf("said %q, want %q", said.Names(), want)
	}
	if err := os.MkdirAll(e.Dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.Dirs.ConfigFile(), []byte("layers = [\"nix\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.Set([]string{"docker"}); msg.NamesOf(err) != "set_in_config file="+e.Dirs.ConfigFile()+" key=layers" {
		t.Errorf("Set with config.toml's: %v", msg.NamesOf(err))
	}
	c.Pick = func(msg.M, []PickPart) []PickPart { t.Error("asked"); return nil }
	if err := c.Ask(); err != nil {
		t.Fatal(err)
	}
	if got := said.Names()[len(said.Names())-1]; got != "setup_in_config file="+e.Dirs.ConfigFile()+" key=layers" {
		t.Errorf("said %q", got)
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
	if get(st, keyToolNames) != "go" || strings.Contains(get(st, KeyTools), "claude") {
		t.Errorf("state tool_names=%s tools=%s", get(st, keyToolNames), get(st, KeyTools))
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
	had := get(st, KeyTools)
	if err := tl.Update(t.Context(), Options{}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if st, _ = state.ReadVM(e.Dirs, vm); !strings.HasPrefix(had, "claude-") || get(st, KeyTools) != had {
		t.Errorf("Update after adding: tools %q, were %q", get(st, KeyTools), had)
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

// A setting that a running VM gets only when it next starts offers to
// restart it, where a question with a default shows: yes restarts it as
// devvm restart does (stopped, started, a shell opened), and the setting
// applies; no, no terminal and --no-input each say when it applies, the
// last two without asking.
func TestRestartToApply(t *testing.T) {
	for _, c := range []struct {
		cmd     string
		set     func(e *Env) error
		later   string // its message's names, before vm=
		applied func(st *state.VM) bool
	}{
		{
			"devvm size --cpus 2", func(e *Env) error { return Settings{e}.SetSize(t.Context(), Size{CPUs: "2"}, false) },
			"size_next_start", nil,
		},
		{
			"devvm network on", func(e *Env) error { return Net{e}.SetNetwork(t.Context(), true) },
			"net_set_running state=on", nil,
		},
		{
			"devvm network off", func(e *Env) error { return Net{e}.SetNetwork(t.Context(), false) },
			"net_set_running state=off", nil,
		},
		{
			"devvm ssh-agent on", func(e *Env) error { return Settings{e}.SetSSHAgent(t.Context(), sshagent.On) },
			"agent_set_running mode=on", nil,
		},
		{
			"devvm git-protect strict", func(e *Env) error { return Settings{e}.SetGitProtect(t.Context(), gitprotect.Strict) },
			"git_protect_next_stop mode=strict",
			func(st *state.VM) bool { return gitprotect.Mode(st) == gitprotect.Strict },
		},
	} {
		for _, a := range []struct {
			name             string
			terminal, canAsk bool
			yes              bool
		}{
			{"yes", true, true, true},
			{"no", true, true, false},
			{"no terminal", false, false, true},
			{"--no-input", true, false, true},
		} {
			// The name is a folder's (t.TempDir): no spaces or commas.
			name := strings.Join(strings.Fields(c.cmd+" "+a.name), "-")
			t.Run(name, func(t *testing.T) {
				e, f, execs := upEnv(t, allNative())
				f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
				f.ExecFunc = noDirect
				o := Options{NoNetworkPolicy: true}
				if c.cmd == "devvm network off" {
					o = Options{NetworkPolicy: true}
				}
				st := existingVM(t, e, o)
				vm := st.Name()
				in := f.Instances[vm]
				in.Size = backend.Size{CPUs: 4, MemoryGiB: 8, DiskGiB: 100}
				f.Instances[vm] = in
				var said msg.Recorder
				e.Info, e.Warn = said.Add, said.Add
				asked := answers(t, e, a.yes)
				e.Terminal = func() bool { return a.terminal }
				e.CanAsk = func() bool { return a.canAsk }
				before, shells := len(f.Calls()), len(*execs)
				if err := c.set(e); err != nil {
					t.Fatal(msg.NamesOf(err))
				}
				ms := f.Methods()[before:]
				i, j := slices.Index(ms, "Stop"), slices.Index(ms, "Start")
				restarted := i >= 0 && j > i && len(*execs) == shells+1
				later := c.later + " vm=" + vm
				switch {
				case a.canAsk && !slices.Equal(*asked, []string{"restart_now vm=" + vm}):
					t.Errorf("asked %q", *asked)
				case !a.canAsk && len(*asked) != 0:
					t.Errorf("asked %q without a question that shows", *asked)
				}
				if want := a.canAsk && a.yes; restarted != want {
					t.Errorf("restarted = %v, want %v: calls %q", restarted, want, ms)
				}
				if slices.Contains(said.Names(), later) == restarted {
					t.Errorf("said %q", said.Names())
				}
				if c.applied != nil && c.applied(reread(t, e, vm)) != restarted {
					t.Errorf("applied = %v", !restarted)
				}
			})
		}
	}
}
