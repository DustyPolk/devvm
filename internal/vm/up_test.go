package vm

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/project"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// A new VM: built, created with the folder, its state written, started
// without network policy, and the backend's shell run.
func TestUp(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Size: Size{CPUs: "2"}, Ports: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	ms := f.Methods()
	want := []string{"Problems", "Inspect", "Images", "BuildImage", "CreateVM", "Start", "Running", "Exec", "List", "SSH", "Shell"}
	if !slices.Equal(ms, want) {
		t.Errorf("calls = %q, want %q", ms, want)
	}
	create, start := call(f, "CreateVM"), call(f, "Start")
	spec, _ := create.Args[1].(backend.VMSpec)
	if spec.Project != e.Dir || spec.Size.CPUs != 2 || !strings.HasPrefix(spec.Name, "devvm-web-") {
		t.Errorf("CreateVM spec = %+v", spec)
	}
	// Every VM has the share, read-only, even with no tools of its own.
	if spec.Tools != e.Dirs.Tools() || spec.ToolsAt != tools.ShareAt {
		t.Errorf("CreateVM's tools share = %q at %q", spec.Tools, spec.ToolsAt)
	}
	if got := staged(t, f); got["tools"] != "" || !strings.HasPrefix(got["sync.sh"], "#!/bin/bash") || got["tools.sh"] == "" {
		t.Errorf("staged = %q", got)
	}
	if len(*execs) != 1 || (*execs)[0].Path != "/bin/shell" {
		t.Errorf("exec = %+v", *execs)
	}
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if st.MadeBy != testVersion {
		t.Errorf("made by %q, want %q", st.MadeBy, testVersion)
	}
	// Without network policy too, its ssh port is devvm's: recorded, made
	// with it and started on it.
	c, _ := start.Args[2].(backend.Confinement)
	if sp := get(st, keySSHPort); c.Net != backend.NetDirect || sp == "" || strconv.Itoa(c.SSHPort) != sp || strconv.Itoa(spec.SSHPort) != sp {
		t.Errorf("Start confinement = %+v, VMSpec's ssh port %d, state's %q", c, spec.SSHPort, sp)
	}
	for k, v := range map[string]string{
		keyImage: "base", keyDir: e.Dir, keyBackend: "fake", KeyNet: "off", KeyCPUs: "2",
		KeyPorts: "none", keyGitProtect: "on",
	} {
		if got := get(st, k); got != v {
			t.Errorf("state %s = %q, want %q", k, got, v)
		}
	}
	// It exists now, and runs: opened as it is, set up again.
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if ms := f.Methods()[before:]; slices.Contains(ms, "Start") || slices.Contains(ms, "CreateVM") || !slices.Contains(ms, "Exec") {
		t.Errorf("calls again = %q", ms)
	}
	if len(*execs) != 2 {
		t.Errorf("exec = %+v", *execs)
	}
}

// noDirect makes the fake VM's direct connections fail, as confined.
func noDirect(_ string, argv []string, _ backend.ExecOpts) error {
	if argv[0] == "curl" {
		return errors.New("curl: (7) Failed to connect")
	}
	return nil
}

// With network policy: its proxy runs before the VM starts confined, with
// the VM's key as the proxy's password, and the self-check passes before
// the setup and the shell.
func TestUpNetworkPolicy(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	f.LifelineFile = filepath.Join(e.Dirs.Lima, "vm", "ha.pid")
	f.ExecFunc = noDirect
	if err := (Launch{e}).Up(t.Context(), Options{NetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	// Its forwarder, once it runs: how to reach it, its host keys for its
	// start (asked which start, read, then asked again).
	want := []string{"Problems", "Inspect", "Images", "BuildImage", "CreateVM", "Running", "Start", "ForwardTarget", "Running", "Exec", "Running", "Running", "Exec", "Exec", "List", "SSH", "Shell"}
	if ms := f.Methods(); !slices.Equal(ms, want) {
		t.Errorf("calls = %q, want %q", ms, want)
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := call(f, "Start").Args[2].(backend.Confinement)
	if c.Net != backend.NetProxy || c.ProxyPassword != get(st, keyProxyKey) ||
		strconv.Itoa(c.ProxyPort) != get(st, keyProxyPort) || strconv.Itoa(c.SSHPort) != get(st, keySSHPort) {
		t.Errorf("Start confinement = %+v", c)
	}
	if get(st, KeyNet) != "on" || get(st, keyCheck) != "ok" {
		t.Errorf("state net=%q net_check=%q", get(st, KeyNet), get(st, keyCheck))
	}
	if _, ok := e.Proxy.PID(spec.Name); !ok {
		t.Error("its proxy doesn't run")
	}
	if len(*execs) != 1 {
		t.Errorf("exec = %+v", *execs)
	}
}

// A VM devvm started confined that still reaches the internet directly is
// stopped: no setup, no shell.
func TestUpNetworkPolicyFailsClosed(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	err := Launch{e}.Up(t.Context(), Options{NetworkPolicy: true})
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	if msg.NamesOf(err) != "net_direct_route vm="+spec.Name {
		t.Fatalf("Up = %v", msg.NamesOf(err))
	}
	want := []string{"Problems", "Inspect", "Images", "BuildImage", "CreateVM", "Running", "Start", "ForwardTarget", "Running", "Exec", "Running", "Running", "Running", "Running", "Exec", "Running", "Stop"}
	if ms := f.Methods(); !slices.Equal(ms, want) || len(*execs) > 0 {
		t.Errorf("calls = %q, want %q; execs %+v", ms, want, *execs)
	}
	if f.Instances[spec.Name].Status != backend.StatusStopped {
		t.Error("left running")
	}
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if get(st, keyCheck) != "unenforced" {
		t.Errorf("net_check = %q", get(st, keyCheck))
	}
}

// The network choice is in the state before the VM is made: one whose
// creation fails later keeps it.
func TestUpKeepsTheNetworkChoice(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.Errs = map[string]error{"CreateVM": errors.New("clone failed")}
	if err := (Launch{e}).Up(t.Context(), Options{NetworkPolicy: true}); err == nil {
		t.Fatal("Up succeeded")
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if get(st, KeyNet) != "on" || slices.Contains(f.Methods(), "Start") {
		t.Errorf("net=%q, calls %q", get(st, KeyNet), f.Methods())
	}
}

// A backend that can't confine a VM to its proxy is refused before
// anything is built, never started open.
func TestUpRequiresEgressProxy(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureEgressProxy] = backend.Emulated(false)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{NetworkPolicy: true})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Errorf("calls = %q", f.Methods())
	}
	// Without network policy it doesn't need it.
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Errorf("Up without network policy = %v", msg.NamesOf(err))
	}
}

// On a backend without egress-proxy, a VM made without either flag is
// made without network policy, warned, and --network-policy is refused.
func TestUpWithoutEgressProxy(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureEgressProxy)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{NetworkPolicy: true})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=egress-proxy vm=devvm-web-") ||
		slices.Contains(f.Methods(), "BuildImage") {
		t.Errorf("Up --network-policy = %v, calls %q", got, f.Methods())
	}
	var warned msg.Recorder
	e.Warn = warned.Add
	if err := (Launch{e}).Up(t.Context(), Options{}); err != nil {
		t.Fatalf("Up = %v", msg.NamesOf(err))
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if get(st, KeyNet) != "off" || !slices.Contains(warned.Names(), "net_off_backend backend=fake vm="+spec.Name) {
		t.Errorf("net=%q, warned %q", get(st, KeyNet), warned.Names())
	}
}

// --agents goes with the folder's toolchains; the shared tools are built
// from base, listed in the VM's state with base's libc, and sent to the VM
// to mount. The image is base: tools aren't layers.
func TestUpTools(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	if err := os.WriteFile(filepath.Join(e.Dir, "package.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var said msg.Recorder
	e.Info = said.Add
	if err := (Launch{e}).Up(t.Context(), Options{Agents: &[]string{"claude"}, NoNetworkPolicy: true}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !slices.ContainsFunc(said.Names(), func(s string) bool { return strings.HasPrefix(s, "new_vm_gets layers=none tools=node claude vm=") }) {
		t.Errorf("said %q", said.Names())
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"Problems", "Inspect", "Images", "BuildImage", "Images", "BuildTools", "CreateVM", "Start", "ForwardTarget", "Running", "Exec", "Running", "Running", "Running", "Exec", "List", "SSH", "Shell"}) {
		t.Errorf("calls = %q", ms)
	}
	lock, err := os.ReadFile(e.Dirs.ToolLock())
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, tool := range []string{"node", "pnpm", "bun", "claude"} {
		file, err := e.Tools.File(tools.Lock(lock), tool, tools.Target{Libc: "gnu"})
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if get(st, keyImage) != "base" || !slices.Equal(tools.Names(st), []string{"node", "claude"}) || get(st, KeyLibc) != "gnu" || get(st, KeyTools) != strings.Join(files, " ") {
		t.Errorf("state image=%q tool_names=%q libc=%q tools=%q", get(st, keyImage), tools.Names(st), get(st, KeyLibc), get(st, KeyTools))
	}
	// Made from base's image: the tools aren't layers.
	if img, _ := call(f, "CreateVM").Args[0].(string); img != call(f, "BuildImage").Args[0].(backend.ImageSpec).Key {
		t.Errorf("CreateVM from %q, not base's image", img)
	}
	if got := staged(t, f)["tools"]; got != string(tools.Mounts(files)) || !strings.HasPrefix(got, files[0]+" node 24.21.0\n") {
		t.Errorf("staged tools = %q", got)
	}
	for _, file := range files {
		if !(tools.Share{Env: e.Tools}).Built(file) {
			t.Errorf("%s isn't in the share", file)
		}
	}
}

// A new VM's defaults: the toolchains the folder's files call for, devvm
// setup's agents, and no layers, the nix layer a flake.nix calls for
// included; each flag replaces its part, none with an empty list. Without
// a terminal one line says what the VM gets.
func TestUpParts(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		o                    Options
		img, tools, gets     string
		setup                []string // devvm setup's agents; nil for unset
		setupLayers          []string // devvm setup's layers; nil for unset
		warned               string   // what's warned, when anything is
		withFlake, withLayer bool
	}{
		{name: "setup's layers", setupLayers: []string{"nix", "mine"}, img: "nix+mine", tools: "node go", gets: "layers=nix mine tools=node go", withLayer: true},
		{name: "--layers replaces setup's", o: Options{Layers: &[]string{"docker"}}, setupLayers: []string{"nix"}, img: "docker", tools: "node go", gets: "layers=docker tools=node go"},
		{name: "--layers none", o: Options{Layers: &[]string{}}, setupLayers: []string{"nix"}, img: "base", tools: "node go", gets: "layers=none tools=node go"},
		{
			name: "setup's layer gone", setupLayers: []string{"mine", "nix"}, img: "nix", tools: "node go", gets: "layers=nix tools=node go",
			warned: "default_layer_gone dir=LAYERS layer=mine vm=VM",
		},
		{name: "a gone layer --layers replaces", o: Options{Layers: &[]string{}}, setupLayers: []string{"mine"}, img: "base", tools: "node go", gets: "layers=none tools=node go"},
		{name: "the defaults", img: "base", tools: "node go", gets: "layers=none tools=node go", withFlake: true},
		{name: "setup's agents", setup: []string{"codex"}, img: "base", tools: "node go codex", gets: "layers=none tools=node go codex"},
		{name: "setup's none", setup: []string{}, img: "base", tools: "node go", gets: "layers=none tools=node go"},
		{name: "--tools", o: Options{Toolchains: &[]string{"python"}}, img: "base", tools: "python", gets: "layers=none tools=python"},
		{name: "--tools none", o: Options{Toolchains: &[]string{}}, setup: []string{"claude"}, img: "base", tools: "claude", gets: "layers=none tools=claude"},
		{name: "--agents none", o: Options{Agents: &[]string{}}, setup: []string{"claude"}, img: "base", tools: "node go", gets: "layers=none tools=node go"},
		{name: "--layers", o: Options{Layers: &[]string{"mine", "nix"}}, img: "mine+nix", tools: "node go", gets: "layers=mine nix tools=node go", withLayer: true},
		{name: "all none", o: Options{Toolchains: &[]string{}, Agents: &[]string{}, Layers: &[]string{}}, img: "base", gets: "layers=none tools=none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, f, _ := upEnv(t, allNative())
			for _, file := range []string{"package.json", "go.mod"} {
				if err := os.WriteFile(filepath.Join(e.Dir, file), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.withFlake {
				if err := os.WriteFile(filepath.Join(e.Dir, "flake.nix"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.withLayer {
				if err := os.MkdirAll(e.Dirs.Layers(), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(e.Dirs.Layers(), "mine.sh"), []byte("#!/bin/bash\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setup != nil {
				if _, err := settings.Save(e.Dirs, settings.Values{Agents: &tc.setup}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setupLayers != nil {
				if _, err := settings.Save(e.Dirs, settings.Values{Layers: &tc.setupLayers}); err != nil {
					t.Fatal(err)
				}
			}
			var said, warned msg.Recorder
			e.Info, e.Warn = said.Add, warned.Add
			o := tc.o
			o.NoNetworkPolicy, o.Ports = true, "none"
			if err := (Launch{e}).Up(t.Context(), o); err != nil {
				t.Fatal(msg.NamesOf(err))
			}
			spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
			st, _ := state.ReadVM(e.Dirs, spec.Name)
			if get(st, keyImage) != tc.img || get(st, keyToolNames) != tc.tools {
				t.Errorf("image=%q tool_names=%q, want %q and %q", get(st, keyImage), get(st, keyToolNames), tc.img, tc.tools)
			}
			if !slices.Contains(said.Names(), "new_vm_gets "+tc.gets+" vm="+spec.Name) {
				t.Errorf("said %q, want new_vm_gets %s", said.Names(), tc.gets)
			}
			want := strings.NewReplacer("LAYERS", e.Dirs.Layers(), "VM", spec.Name).Replace(tc.warned)
			var layerWarnings []string
			for _, w := range warned.Names() {
				if strings.HasPrefix(w, "default_layer") {
					layerWarnings = append(layerWarnings, w)
				}
			}
			if got := strings.Join(layerWarnings, "\n"); got != want {
				t.Errorf("warned %q, want %q", got, want)
			}
		})
	}
}

// A layer of devvm setup's that the backend can't run fails before
// anything is built, naming the layer; --layers without it makes the VM.
func TestUpSetupLayerLacks(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDockerLayer)
	e, f, _ := upEnv(t, caps)
	if _, err := settings.Save(e.Dirs, settings.Values{Layers: &[]string{"docker"}}); err != nil {
		t.Fatal(err)
	}
	err := Launch{e}.Up(t.Context(), Options{NoNetworkPolicy: true, Ports: "none"})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "default_layer_lacks backend=fake feature=docker-layer layer=docker vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") {
		t.Errorf("calls = %q", ms)
	}
	if err := (Launch{e}).Up(t.Context(), Options{Layers: &[]string{}, NoNetworkPolicy: true, Ports: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
}

// In a terminal without the flags, the picker shows the defaults checked,
// each detected toolchain naming its file and setup's agents and layers
// saying so, the nix layer naming the flake.nix, and the other layers
// unchecked; what's checked is what the VM gets, and no line says it. On
// the first run it asks for setup's agents first.
func TestUpPicker(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	for _, file := range []string{"package.json", "flake.nix"} {
		if err := os.WriteFile(filepath.Join(e.Dir, file), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	e.CanPick = func() bool { return true }
	if _, err := settings.Save(e.Dirs, settings.Values{Layers: &[]string{"nix", "desktop"}}); err != nil {
		t.Fatal(err)
	}
	var asked []string
	var shown []PickPart
	e.PickParts = func(help msg.M, parts []PickPart) []PickPart {
		asked = append(asked, help.ID())
		if help.ID() == "agents_help" {
			parts[0].Rows[1].On = true // codex
			return parts
		}
		shown = parts
		out := make([]PickPart, len(parts))
		for i, p := range parts {
			out[i] = PickPart{Heading: p.Heading, Rows: slices.Clone(p.Rows)}
		}
		out[0].Rows[1].On = true // python
		out[2].Rows[1].On = true // docker
		return out
	}
	var said msg.Recorder
	e.Info = said.Add
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Ports: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
	if !slices.Equal(asked, []string{"agents_help", "pick_new_help"}) {
		t.Fatalf("asked %q", asked)
	}
	if def, _ := settings.Read(e.Dirs); !def.Agents.Chosen || !slices.Equal(def.Agents.Names, []string{"codex"}) {
		t.Errorf("setup's agents = %+v", def.Agents)
	}
	// Each row as its name, its description's names and whether it's on.
	type row struct {
		Name, Desc string
		On         bool
	}
	rows := map[string]row{}
	var headings []string
	for _, p := range shown {
		headings = append(headings, p.Heading.ID())
		for _, r := range p.Rows {
			rows[r.Name] = row{r.Name, r.Desc.Names(), r.On}
		}
	}
	if !slices.Equal(headings, []string{"pick_toolchains", "pick_agents", "pick_layers"}) {
		t.Errorf("headings %q", headings)
	}
	var layers []string
	for _, r := range shown[2].Rows {
		layers = append(layers, r.Name)
	}
	if !slices.Equal(layers, []string{"desktop", "docker", "nix"}) {
		t.Errorf("layers %q", layers)
	}
	for name, want := range map[string]row{
		"desktop": {Name: "desktop", Desc: "pick_from_setup", On: true},
		"node":    {Name: "node", Desc: "pick_called_for file=package.json", On: true},
		"python":  {Name: "python"},
		"codex":   {Name: "codex", Desc: "pick_from_setup", On: true},
		"claude":  {Name: "claude"},
		"docker":  {Name: "docker", Desc: "pick_layer_docker"},
		"nix":     {Name: "nix", Desc: "pick_called_for file=flake.nix", On: true},
	} {
		if rows[name] != want {
			t.Errorf("row %s = %+v, want %+v", name, rows[name], want)
		}
	}
	// setup's layers keep their order, bottom first, under the one added.
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if get(st, keyImage) != "nix+desktop+docker" || get(st, keyToolNames) != "node python codex" {
		t.Errorf("image=%q tool_names=%q", get(st, keyImage), get(st, keyToolNames))
	}
	if slices.ContainsFunc(said.Names(), func(s string) bool { return strings.HasPrefix(s, "new_vm_gets") }) {
		t.Errorf("said %q", said.Names())
	}
}

// Any of the flags skips the picker.
func TestUpFlagsSkipPicker(t *testing.T) {
	e, _, _ := upEnv(t, allNative())
	e.CanPick = func() bool { return true }
	if _, err := settings.Save(e.Dirs, settings.Values{Agents: &[]string{}}); err != nil {
		t.Fatal(err)
	}
	e.PickParts = func(help msg.M, parts []PickPart) []PickPart {
		t.Errorf("asked %s", help.Names())
		return parts
	}
	if err := (Launch{e}).Up(t.Context(), Options{Layers: &[]string{}, NoNetworkPolicy: true, Ports: "none"}); err != nil {
		t.Fatal(msg.NamesOf(err))
	}
}

// A backend without a feature the VM needs is refused before anything is
// built.
func TestUpRequires(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureProjectMount)
	e, f, execs := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{NoNetworkPolicy: true})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=project-mount vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") || len(*execs) > 0 {
		t.Errorf("calls = %q, execs %+v", ms, *execs)
	}
}

// Below the free space a new VM wants, nothing is built or made, and the
// VM has no state; with enough for the VM but not base, base's build is
// refused before it starts.
func TestUpRefusesLowSpace(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	reg, err := backend.NewRegistry(backendtest.Stored{Fake: f, Dirs: []string{e.Dirs.Lima}})
	if err != nil {
		t.Fatal(err)
	}
	reg.NewVMs = "fake"
	e.Registry = reg
	for _, tc := range []struct {
		free uint64
		want string
	}{
		{4<<30 - 1, "disk_low dir=" + e.Dirs.Lima + " free=3.9 name=devvm-web-"},
		{12<<30 - 1, "disk_low dir=" + e.Dirs.Lima + " free=11.9 name=base need=12"},
	} {
		e.FreeSpace = func(string) (uint64, error) { return tc.free, nil }
		err := Launch{e}.Up(t.Context(), Options{NoNetworkPolicy: true, Ports: "none"})
		if got := msg.NamesOf(err); !strings.HasPrefix(got, tc.want) {
			t.Errorf("with %d free: Up = %v, want %s…", tc.free, got, tc.want)
		}
		if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") || len(*execs) > 0 {
			t.Errorf("with %d free: calls = %q, execs %+v", tc.free, ms, *execs)
		}
		if vms, _ := os.ReadDir(e.Dirs.VMs()); len(vms) > 0 {
			t.Errorf("with %d free: a VM's state is left", tc.free)
		}
	}
}

// The docker layer needs docker-layer of the backend, before anything is
// built.
func TestUpRequiresDockerLayer(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDockerLayer)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{Layers: &[]string{"docker"}, NoNetworkPolicy: true, Ports: "none"})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=docker-layer vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") {
		t.Errorf("calls = %q", ms)
	}
}

// A disk size, the default's or --disk's, needs disk of the backend,
// before anything is built.
func TestUpDiskNeedsDisk(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDisk)
	e, f, _ := upEnv(t, caps)
	saveSize(t, e.Dirs, Size{CPUs: "2", Disk: "150"})
	err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true, Ports: "none"})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=disk ") {
		t.Errorf("the default's: %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") {
		t.Errorf("calls = %q", ms)
	}
	e2, _, _ := upEnv(t, caps)
	err = (Launch{e2}).Up(t.Context(), Options{NoNetworkPolicy: true, Ports: "none", Size: Size{Disk: "150"}})
	if got := msg.NamesOf(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=disk ") {
		t.Errorf("--disk: %v", got)
	}
}

// Only an unfinished VM of this folder's own is offered a delete and
// remake, and only when the question can be asked: a finished VM never.
func TestRemakeUnfinished(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		finished, ask   bool
		sub             string
		canAsk, offered bool
	}{
		{name: "finished", finished: true, canAsk: true, ask: true},
		{name: "unfinished, yes", canAsk: true, ask: true, offered: true},
		{name: "unfinished, no", canAsk: true},
		{name: "no question", ask: true},
		{name: "parent's", sub: "/p/sub", canAsk: true, ask: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config")}
			st := vmState(t, d, "devvm-p-00000000000000", func(st *state.VM) error {
				if !tc.finished {
					st.Created = time.Time{}
				}
				return nil
			})
			asked := false
			e := &Env{
				CanAsk: func() bool { return tc.canAsk },
				Ask:    func(msg.M) bool { asked = true; return tc.ask },
			}
			got := remakeUnfinished(e, &found{f: project.Folder{Name: st.Name()}, st: st, sub: tc.sub})
			if got != tc.offered {
				t.Errorf("remakeUnfinished = %v, want %v", got, tc.offered)
			}
			if asked != (!tc.finished && tc.canAsk && tc.sub == "") {
				t.Errorf("asked = %v", asked)
			}
		})
	}
}
