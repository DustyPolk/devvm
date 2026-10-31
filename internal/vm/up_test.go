package vm

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

// A new VM: built, created with the folder, its state written, started
// without network policy, and the backend's shell run.
func TestUp(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Size: Size{CPUs: "2"}, Ports: "none"}); err != nil {
		t.Fatal(names(err))
	}
	ms := f.Methods()
	want := []string{"Check", "Inspect", "Images", "BuildImage", "CreateVM", "Start", "Exec", "List", "SSH", "Shell"}
	if !slices.Equal(ms, want) {
		t.Errorf("calls = %q, want %q", ms, want)
	}
	create, start := call(f, "CreateVM"), call(f, "Start")
	spec, _ := create.Args[1].(backend.VMSpec)
	if spec.Project != e.Dir || spec.Size.CPUs != 2 || !reflect.DeepEqual(spec.Ports, backend.Ports{}) || !strings.HasPrefix(spec.Name, "devvm-web-") {
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
	// Without network policy too, its ssh port is devvm's: recorded, made
	// with it and started on it.
	c, _ := start.Args[1].(backend.Confinement)
	if sp := st.Get(netpolicy.KeySSHPort); c.Net != backend.NetDirect || sp == "" || strconv.Itoa(c.SSHPort) != sp || strconv.Itoa(spec.SSHPort) != sp {
		t.Errorf("Start confinement = %+v, VMSpec's ssh port %d, state's %q", c, spec.SSHPort, sp)
	}
	for k, v := range map[string]string{
		state.KeyImage: "base", state.KeyDir: e.Dir, state.KeyBackend: "fake", KeyNet: "off", KeyCPUs: "2",
		KeyPorts: "none", gitprotect.KeyGitProtect: "on",
	} {
		if got := st.Get(k); got != v {
			t.Errorf("state %s = %q, want %q", k, got, v)
		}
	}
	// It exists now, and runs: opened as it is, set up again.
	before := len(f.Calls())
	if err := (Launch{e}).Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
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
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
	}
	want := []string{"Check", "Inspect", "Images", "BuildImage", "CreateVM", "Start", "Confined", "Exec", "Exec", "List", "SSH", "Shell"}
	if ms := f.Methods(); !slices.Equal(ms, want) {
		t.Errorf("calls = %q, want %q", ms, want)
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := call(f, "Start").Args[1].(backend.Confinement)
	if c.Net != backend.NetProxy || c.ProxyUser != "devvm" || c.ProxyPassword != st.Get(netpolicy.KeyProxyKey) ||
		strconv.Itoa(c.ProxyPort) != st.Get(netpolicy.KeyProxyPort) || strconv.Itoa(c.SSHPort) != st.Get(netpolicy.KeySSHPort) {
		t.Errorf("Start confinement = %+v", c)
	}
	if st.Get(KeyNet) != "on" || st.Get(netpolicy.KeyCheck) != "ok" {
		t.Errorf("state net=%q net_check=%q", st.Get(KeyNet), st.Get(netpolicy.KeyCheck))
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
	err := Launch{e}.Up(t.Context(), Options{Image: "base", NetworkPolicy: true})
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	if names(err) != "net_direct_route vm="+spec.Name {
		t.Fatalf("Up = %v", names(err))
	}
	want := []string{"Check", "Inspect", "Images", "BuildImage", "CreateVM", "Start", "Confined", "Exec", "Confined", "Stop"}
	if ms := f.Methods(); !slices.Equal(ms, want) || len(*execs) > 0 {
		t.Errorf("calls = %q, want %q; execs %+v", ms, want, *execs)
	}
	if f.Instances[spec.Name].Status != backend.StatusStopped {
		t.Error("left running")
	}
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if st.Get(netpolicy.KeyCheck) != "unenforced" {
		t.Errorf("net_check = %q", st.Get(netpolicy.KeyCheck))
	}
}

// The network choice is in the state before the VM is made: one whose
// creation fails later keeps it.
func TestUpKeepsTheNetworkChoice(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	f.Errs = map[string]error{"CreateVM": errors.New("clone failed")}
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NetworkPolicy: true}); err == nil {
		t.Fatal("Up succeeded")
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if st.Get(KeyNet) != "on" || slices.Contains(f.Methods(), "Start") {
		t.Errorf("net=%q, calls %q", st.Get(KeyNet), f.Methods())
	}
}

// A backend that can't confine a VM to its proxy is refused before
// anything is built, never started open.
func TestUpRequiresEgressProxy(t *testing.T) {
	caps := allNative()
	caps[backend.FeatureEgressProxy] = backend.Emulated(false)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{Image: "base", NetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Errorf("calls = %q", f.Methods())
	}
	// Without network policy it doesn't need it.
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true}); err != nil {
		t.Errorf("Up without network policy = %v", names(err))
	}
}

// On a backend without egress-proxy, a VM made without either flag is
// made without network policy, warned, and --network-policy is refused.
func TestUpWithoutEgressProxy(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureEgressProxy)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{Image: "base", NetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=egress-proxy vm=devvm-web-") ||
		slices.Contains(f.Methods(), "BuildImage") {
		t.Errorf("Up --network-policy = %v, calls %q", got, f.Methods())
	}
	var warned []string
	e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base"}); err != nil {
		t.Fatalf("Up = %v", names(err))
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	st, _ := state.ReadVM(e.Dirs, spec.Name)
	if st.Get(KeyNet) != "off" || !slices.Contains(warned, "net_off_backend backend=fake vm="+spec.Name) {
		t.Errorf("net=%q, warned %q", st.Get(KeyNet), warned)
	}
}

// Agents go on top of the folder's toolchains; the shared tools are built
// from base, listed in the VM's state with base's libc, and sent to the VM
// to mount.
func TestUpTools(t *testing.T) {
	e, f, _ := upEnv(t, allNative())
	if err := os.WriteFile(filepath.Join(e.Dir, "package.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Launch{e}).Up(t.Context(), Options{Image: "claude", NoNetworkPolicy: true}); err != nil {
		t.Fatal(names(err))
	}
	if ms := f.Methods(); !slices.Equal(ms, []string{"Check", "Inspect", "Images", "BuildImage", "BuildTools", "CreateVM", "Start", "Exec", "List", "SSH", "Shell"}) {
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
	if st.Get(state.KeyImage) != "node+claude" || st.Get(KeyLibc) != "gnu" || st.Get(KeyTools) != strings.Join(files, " ") {
		t.Errorf("state image=%q libc=%q tools=%q", st.Get(state.KeyImage), st.Get(KeyLibc), st.Get(KeyTools))
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

// Only agents, or nothing, need the folder's toolchains under them; a
// toolchain, a layer or base is a choice of its own.
func TestNeedsDetection(t *testing.T) {
	for in, want := range map[string]bool{
		"": true, "claude": true, "claude+codex": true,
		"node+claude": false, "python+claude": false, "base": false, "base+claude": false, "foo": false,
	} {
		if got := needsDetection(in); got != want {
			t.Errorf("needsDetection(%q) = %v", in, got)
		}
	}
}

// A backend without a feature the VM needs is refused before anything is
// built.
func TestUpRequires(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureProjectMount)
	e, f, execs := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=project-mount vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") || len(*execs) > 0 {
		t.Errorf("calls = %q, execs %+v", ms, *execs)
	}
}

// stored is a fake whose disks are in dirs (backend.Storage).
type stored struct {
	*backendtest.Fake
	dirs []string
}

func (s stored) DiskDirs() []string { return s.dirs }

func (s stored) Floors() backend.Floors {
	return backend.Floors{Base: 12 << 30, Image: 8 << 30, VM: 4 << 30, Tools: 3 << 30, PerTool: 1 << 30}
}

// Below the free space a new VM wants, nothing is built or made, and the
// VM has no state; with enough for the VM but not base, base's build is
// refused before it starts.
func TestUpRefusesLowSpace(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	reg, err := backend.NewRegistry(stored{f, []string{e.Dirs.Lima}})
	if err != nil {
		t.Fatal(err)
	}
	e.Registry = reg
	for _, tc := range []struct {
		free uint64
		want string
	}{
		{4<<30 - 1, "disk_low dir=" + e.Dirs.Lima + " free=3.9 name=devvm-web-"},
		{12<<30 - 1, "disk_low dir=" + e.Dirs.Lima + " free=11.9 name=base need=12"},
	} {
		e.FreeSpace = func(string) (uint64, error) { return tc.free, nil }
		err := Launch{e}.Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Ports: "none"})
		if got := names(err); !strings.HasPrefix(got, tc.want) {
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
// built: for a new VM, and for one it's added to.
func TestUpRequiresDockerLayer(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDockerLayer)
	e, f, _ := upEnv(t, caps)
	err := Launch{e}.Up(t.Context(), Options{Image: "docker", NoNetworkPolicy: true, Ports: "none"})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=docker-layer vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") {
		t.Errorf("calls = %q", ms)
	}
	existingVM(t, e, Options{Image: "base", NoNetworkPolicy: true})
	before := len(f.Calls())
	err = Launch{e}.Up(t.Context(), Options{Image: "docker"})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=docker-layer vm=devvm-web-") {
		t.Errorf("adding it: %v", got)
	}
	if ms := f.Methods()[before:]; slices.Contains(ms, "Exec") {
		t.Errorf("calls = %q", ms)
	}
}

// A default disk size doesn't apply on a backend that sizes no disks:
// left out, said; --disk is refused.
func TestUpDefaultDiskUnused(t *testing.T) {
	caps := allNative()
	delete(caps, backend.FeatureDisk)
	e, f, _ := upEnv(t, caps)
	if err := state.WriteSize(e.Dirs, state.Size{CPUs: "2", Disk: "150"}); err != nil {
		t.Fatal(err)
	}
	var warned []string
	e.Warn = func(m msg.M) { warned = append(warned, m.Names()) }
	if err := (Launch{e}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Ports: "none"}); err != nil {
		t.Fatal(names(err))
	}
	spec, _ := call(f, "CreateVM").Args[1].(backend.VMSpec)
	if spec.Size != (backend.Size{CPUs: 2}) {
		t.Errorf("size = %+v", spec.Size)
	}
	if len(warned) != 1 || !strings.HasPrefix(warned[0], "size_disk_default_unused backend=fake vm=devvm-web-") {
		t.Errorf("warned %q", warned)
	}
	if st, _ := state.ReadVM(e.Dirs, spec.Name); st.Get(KeyDisk) != "" || st.Get(KeyCPUs) != "2" {
		t.Errorf("state disk=%q cpus=%q", st.Get(KeyDisk), st.Get(KeyCPUs))
	}
	e2, _, _ := upEnv(t, caps)
	err := (Launch{e2}).Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Ports: "none", Size: Size{Disk: "150"}})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=disk ") {
		t.Errorf("--disk: %v", got)
	}
}
