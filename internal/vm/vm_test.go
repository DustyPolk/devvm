package vm

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/gitprotect"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/netpolicy"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/secrets"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/tools"
)

func names(err error) string {
	if m, ok := err.(msg.M); ok { //nolint:errorlint // messages aren't wrapped
		return m.Names()
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestParseSizes(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for _, tc := range []struct{ in, want, err string }{
		{"6", "6", ""},
		{"1.5", "", `cpus_not_number value="1.5"`},
		{"0", "", `cpus_not_number value="0"`},
		{"many", "", `cpus_not_number value="many"`},
	} {
		got, err := ParseCPUs(tc.in)
		if got != tc.want || names(err) != tc.err {
			t.Errorf("ParseCPUs(%q) = %q, %v", tc.in, got, names(err))
		}
	}
	for _, tc := range []struct{ in, want, err string }{
		{"16", "16", ""},
		{"16G", "16", ""},
		{"16g", "16", ""},
		{"200GiB", "200", ""},
		{"1.5", "1.5", ""},
		{"0.5", "0.5", ""},
		{"0", "", `size_not_gib flag=--memory value="0"`},
		{"16M", "", `size_not_gib flag=--memory value="16M"`},
		{"lots", "", `size_not_gib flag=--memory value="lots"`},
		{"1$(touch x)", "", `size_not_gib flag=--memory value="1$(touch x)"`},
	} {
		got, err := ParseGiB("--memory", tc.in)
		if got != tc.want || names(err) != tc.err {
			t.Errorf("ParseGiB(%q) = %q, %v", tc.in, got, names(err))
		}
	}
	s, err := Size{CPUs: "6", Memory: "12G", Disk: "150"}.Parse()
	if err != nil || s != (Size{CPUs: "6", Memory: "12", Disk: "150"}) {
		t.Errorf("Parse = %+v, %v", s, err)
	}
	if _, err := (Size{Disk: "x"}).Parse(); names(err) != `size_not_gib flag=--disk value="x"` {
		t.Errorf("Parse with a bad disk: %v", names(err))
	}
}

// The flags, else the defaults, else nothing (the image's).
func TestSizeOr(t *testing.T) {
	got := Size{CPUs: "6"}.Or(Size{CPUs: "2", Memory: "16"})
	if got != (Size{CPUs: "6", Memory: "16"}) {
		t.Errorf("Or = %+v", got)
	}
}

func TestSizeCheck(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	h := Host{CPUs: 8, MemoryGiB: 32}
	for _, tc := range []struct {
		s   Size
		err string
	}{
		{Size{CPUs: "16"}, "too_many_cpus have=8 want=16"},
		{Size{Memory: "64"}, "too_much_memory have=32 want=64"},
		{Size{Memory: "32.5"}, "too_much_memory have=32 want=32.5"},
		{Size{Disk: "50"}, "disk_shrink have=100 vm=v want=50"},
		{Size{CPUs: "8", Memory: "32", Disk: "200"}, ""},
		{Size{}, ""},
	} {
		if err := tc.s.Check(h, 100, "v"); names(err) != tc.err {
			t.Errorf("Check(%+v) = %v, want %s", tc.s, names(err), tc.err)
		}
	}
	if b := (Size{CPUs: "2", Memory: "1.5", Disk: "120"}).Backend(); b != (backend.Size{CPUs: 2, MemoryGiB: 1.5, DiskGiB: 120}) {
		t.Errorf("Backend = %+v", b)
	}
}

func TestBaseDisk(t *testing.T) {
	if got := BaseDisk(filepath.Join("..", "..")); got != 100 {
		t.Errorf("BaseDisk(the repo) = %v, want base.yaml's 100", got)
	}
	if got := BaseDisk(t.TempDir()); got != 0 {
		t.Errorf("BaseDisk(no base.yaml) = %v", got)
	}
}

func TestParsePorts(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	for in, want := range map[string]string{
		"all": "all", "none": "none", "5173, 8000-8010,5173": "5173,8000-8010", "0080,3000-3000": "80,3000",
		" all ": "all",
	} {
		if got, err := ParsePorts(in); got != want || err != nil {
			t.Errorf("ParsePorts(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", " ", "0", "65536", "8010-8000", "5173,,80", "abc", "80-", "1-2-3", "123456"} {
		if _, err := ParsePorts(bad); names(err) != "port_invalid value="+`"`+bad+`"` {
			t.Errorf("ParsePorts(%q) = %v", bad, names(err))
		}
	}
}

func TestPortsBackend(t *testing.T) {
	for in, want := range map[string]backend.Ports{
		"":               {All: true},
		"all":            {All: true},
		"none":           {},
		"5173,8000-8010": {Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8010}}},
	} {
		if got := PortsBackend(in); !reflect.DeepEqual(got, want) {
			t.Errorf("PortsBackend(%q) = %+v, want %+v", in, got, want)
		}
	}
}

// upEnv is up on a fake backend, in a temporary sandbox, with a project
// folder named web.
func upEnv(t *testing.T, caps backend.Caps) (*Env, *backendtest.Fake, *[]backend.Command) {
	t.Helper()
	t.Setenv("DEVVM_MESSAGES", "names")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// The run log goes in the sandbox too.
	t.Setenv("XDG_DATA_HOME", root)
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// The shared tools' versions come from the stub mise: the fixture lock.
	t.Setenv("MISE_STUB_LOCK", filepath.Join(repo, "testdata", "mise.lock"))
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config"), Lima: filepath.Join(root, "lima")}
	tl := tools.New(d, repo)
	tl.Getenv = func(k string) string {
		if k == "DEVVM_HOST_MISE" {
			return filepath.Join(repo, "testdata", "stubs", "mise")
		}
		return os.Getenv(k)
	}
	dir := filepath.Join(root, "web")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f := backendtest.New("fake", caps)
	f.ShellCmd = backend.Command{Path: "/bin/shell", Args: []string{"shell", dir}}
	reg, err := backend.NewRegistry(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteBackend(d, "fake"); err != nil {
		t.Fatal(err)
	}
	var execs []backend.Command
	home := filepath.Join(root, "home")
	e := &Env{
		Dirs: d, Root: repo, Registry: reg, Tools: tl, Proxy: testProxy(t, d, repo, home), Dir: dir, Home: home, OS: "darwin",
		Host: func() (Host, error) { return Host{CPUs: 8, MemoryGiB: 32}, nil },
		Protector: func(dir string) *gitprotect.Protector {
			p := gitprotect.New(dir)
			p.Chflags = func(context.Context, string, string) error { return nil }
			return p
		},
		Exec:   func(c backend.Command) error { execs = append(execs, c); return nil },
		Getenv: func(string) string { return "" },
		HostTZ: func() string { return "Europe/Paris" },
		GitConfig: func(_ context.Context, _, key string) string {
			return map[string]string{"user.name": "A User", "user.email": "a@example.com"}[key]
		},
	}
	return e, f, &execs
}

// testProxy runs each VM's "proxy" as a sleep that says it listens, with
// no Keychain and no backups.
func testProxy(t *testing.T, d paths.Dirs, root, home string) *netpolicy.Proxy {
	t.Helper()
	store := secrets.NewStore(d, home)
	store.Keychain, store.Getenv = false, func(string) string { return "" }
	p := netpolicy.NewProxy(d, root, "/devvm", store)
	var mu sync.Mutex
	args := map[int][]string{}
	p.Start = func(path string, a []string, out *os.File) (int, error) {
		cmd := exec.CommandContext(context.Background(), "sleep", "30")
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		t.Cleanup(func() { _ = cmd.Process.Kill(); <-done })
		mu.Lock()
		defer mu.Unlock()
		args[cmd.Process.Pid] = append([]string{path}, a...)
		_, err := fmt.Fprintf(out, "devvm proxy: listening on %s for %s\n", a[2], a[4])
		return cmd.Process.Pid, err
	}
	p.Args = func(pid int) ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		return args[pid], nil
	}
	p.Listening = func(int) bool { return false }
	p.ExcludeFromBackup = func(context.Context, string) error { return nil }
	p.Tick = time.Millisecond
	return p
}

func allNative() backend.Caps {
	caps := backend.Caps{}
	for _, f := range backend.Features {
		caps[f] = backend.Native
	}
	return caps
}

// A new VM: built, created with the folder, its state written, started
// without network policy, and the backend's shell run.
func TestUp(t *testing.T) {
	e, f, execs := upEnv(t, allNative())
	if err := e.Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true, Size: Size{CPUs: "2"}, Ports: "none"}); err != nil {
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
	if c, _ := start.Args[1].(backend.Confinement); c.Net != backend.NetDirect {
		t.Errorf("Start confinement = %+v", c)
	}
	if len(*execs) != 1 || (*execs)[0].Path != "/bin/shell" {
		t.Errorf("exec = %+v", *execs)
	}
	st, err := state.ReadVM(e.Dirs, spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{
		state.KeyImage: "base", state.KeyDir: e.Dir, state.KeyBackend: "fake", KeyNet: "off", KeyCPUs: "2",
		KeyPorts: "none", KeyPortsSet: "none", gitprotect.KeyGitProtect: "on",
	} {
		if got := st.Get(k); got != v {
			t.Errorf("state %s = %q, want %q", k, got, v)
		}
	}
	// It exists now, and runs: opened as it is, set up again.
	before := len(f.Calls())
	if err := e.Up(t.Context(), Options{NoNetworkPolicy: true}); err != nil {
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
	if err := e.Up(t.Context(), Options{Image: "base", NetworkPolicy: true}); err != nil {
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
	err := e.Up(t.Context(), Options{Image: "base", NetworkPolicy: true})
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
	if err := e.Up(t.Context(), Options{Image: "base", NetworkPolicy: true}); err == nil {
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
	err := e.Up(t.Context(), Options{Image: "base", NetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if slices.Contains(f.Methods(), "BuildImage") {
		t.Errorf("calls = %q", f.Methods())
	}
	// Without network policy it doesn't need it.
	if err := e.Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true}); err != nil {
		t.Errorf("Up without network policy = %v", names(err))
	}
}

// call returns the fake's last call of a method.
func call(f *backendtest.Fake, method string) backendtest.Call {
	var last backendtest.Call
	for _, c := range f.Calls() {
		if c.Method == method {
			last = c
		}
	}
	return last
}

// staged returns the files the VM's setup was sent (Exec's input).
func staged(t *testing.T, f *backendtest.Fake) map[string]string {
	t.Helper()
	c := call(f, "Exec")
	if argv, _ := c.Args[1].([]string); !slices.Equal(argv, []string{"bash", "-c", syncCmd}) {
		t.Fatalf("Exec argv = %q", argv)
	}
	o, _ := c.Args[2].(backend.ExecOpts)
	files := map[string]string{}
	r := tar.NewReader(o.Stdin)
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return files
		} else if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		files[h.Name] = string(data)
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
	if err := e.Up(t.Context(), Options{Image: "claude", NoNetworkPolicy: true}); err != nil {
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
		file, err := e.Tools.File(tools.Lock(lock), tool, "gnu")
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
		if !e.Tools.Built(file) {
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
	err := e.Up(t.Context(), Options{Image: "base", NoNetworkPolicy: true})
	if got := names(err); !strings.HasPrefix(got, "backend_lacks backend=fake feature=project-mount vm=devvm-web-") {
		t.Errorf("Up = %v", got)
	}
	if ms := f.Methods(); slices.Contains(ms, "BuildImage") || slices.Contains(ms, "CreateVM") || len(*execs) > 0 {
		t.Errorf("calls = %q, execs %+v", ms, *execs)
	}
}
