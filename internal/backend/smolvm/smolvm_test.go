package smolvm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// execRunner runs the commands as they are: here, the smolvm stub.
type execRunner struct{}

func (execRunner) Run(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }

func (execRunner) Output(cmd *exec.Cmd) ([]byte, error) { return cmd.Output() }

func (execRunner) Attach(cmd *exec.Cmd) error { return cmd.Run() }

// testUI prints nothing, runs commands as they are, counts no steps and
// keeps no cleanups.
type testUI struct{}

func (testUI) Info(msg.M)                                                {}
func (testUI) Warn(msg.M)                                                {}
func (testUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }
func (testUI) WithSteps(ctx context.Context, _ int) context.Context      { return ctx }
func (testUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (testUI) Interrupted() bool                                         { return false }

// sandbox is a smolvm backend on the stub (testdata/stubs/smolvm), first
// on PATH, with devvm's folders in a new temporary folder; its user is u,
// uid 501, on a host with 64 GiB.
type sandbox struct {
	*Smolvm
	stub string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	root := t.TempDir()
	stub := filepath.Join(root, "stub")
	for _, d := range []string{"data", "config", "stub", "src"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stubs, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "stubs"))
	if err != nil {
		t.Fatal(err)
	}
	base, err := os.ReadFile(filepath.Join("..", "..", "..", "base.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "base.yaml"), base, 0o600); err != nil { //nolint:gosec // the test's own folder
		t.Fatal(err)
	}
	t.Setenv("STUB_DIR", stub)
	t.Setenv("STUB_LOG", filepath.Join(stub, "calls"))
	t.Setenv("SMOLVM_MACHINE_NAME", "users-own")
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	d := paths.Dirs{Data: filepath.Join(root, "data"), Config: filepath.Join(root, "config"), Lima: filepath.Join(root, "lima")}
	s := &Smolvm{
		Dirs: d, Runner: execRunner{}, OS: "darwin", Arch: "arm64", Root: filepath.Join(root, "src"),
		Getenv: func(string) string { return "" },
		User:   func() (*user.User, error) { return &user.User{Username: "u", Uid: "501"}, nil },
		Memory: func() (uint64, error) { return 64 << 30, nil },
		Digest: func(context.Context, string) (string, error) {
			return "sha256:" + strings.Repeat("ab", 32), nil
		},
		KVM: "/dev/kvm", Terminal: func() bool { return false },
		FreePort: func() (int, error) { return 61022, nil }, PortFree: func(int) bool { return true },
		Info: func(msg.M) {}, Warn: func(msg.M) {},
		WithSteps: testUI{}.WithSteps, AddCleanup: testUI{}.AddCleanup, Interrupted: testUI{}.Interrupted,
	}
	return sandbox{s, stub}
}

// calls are the stub's calls, without the reads.
func (s sandbox) calls(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.stub, "calls"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		if l != "" && l != "smolvm machine ls --json" && !strings.HasPrefix(l, "smolvm machine data-dir") {
			out = append(out, l)
		}
	}
	return out
}

func (s sandbox) reset(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(s.stub, "calls")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

// machine makes a stub machine: "STATE CPUS MEM STORAGE NET", with this
// devvm's label.
func (s sandbox) machine(t *testing.T, name, fields string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(s.stub, "smolvm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.stub, "smolvm", name), []byte(fields+" "+s.Dirs.Data+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s sandbox) touch(t *testing.T, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.stub, name), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// image makes an image's pack.
func (s sandbox) image(t *testing.T, key string) {
	t.Helper()
	if err := os.MkdirAll(s.Dirs.SmolvmImage(key), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{s.pack(key), s.packFile(key)} {
		if err := os.WriteFile(f, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func names(err error) string {
	var m msg.M
	if !errors.As(err, &m) {
		if err == nil {
			return ""
		}
		return "not a message: " + err.Error()
	}
	return m.Names()
}

// What smolvm has natively, and what waits for an emulation: refused in
// one line naming the VM.
func TestCaps(t *testing.T) {
	s := newSandbox(t)
	caps := s.Caps()
	for _, f := range backend.Features {
		want := !slices.Contains([]backend.Feature{
			backend.FeatureEgressProxy, backend.FeatureSSH, backend.FeaturePortsAll, backend.FeatureAgentToggle,
		}, f)
		none := map[backend.Feature]string{
			backend.FeatureEgressProxy: "emulated", // reviewed
			backend.FeatureSSH:         "emulated", // reviewed
		}[f]
		if none == "" {
			none = "none"
		}
		if caps[f].IsNative() != want || (!want && caps[f].String() != none) {
			t.Errorf("%s: %s", f, caps[f])
		}
	}
	var r backend.Review
	if err := r.Require(s, "devvm-x", backend.Confinement{Net: backend.NetProxy}.Features()...); err != nil {
		t.Errorf("network policy: %v", err)
	}
	if err := r.Require(s, "devvm-x", backend.FeatureSSH); err != nil {
		t.Errorf("ssh: %v", err)
	}
	err := r.Require(s, "devvm-x", backend.VMSpec{Ports: backend.Ports{All: true}}.Features()...)
	if got := names(err); got != "backend_lacks backend=smolvm feature=ports-all vm=devvm-x" {
		t.Errorf("every port: %s", got)
	}
}

// Tool images are uncompressed, named by plain.sh; base's key has
// prepare.sh, shims.sh and sshd.sh.
func TestFormatAndKeyFiles(t *testing.T) {
	s := newSandbox(t)
	if f := s.ToolFormat(); !strings.HasPrefix(f, "plain-") || len(f) != len("plain-")+12 {
		t.Errorf("ToolFormat = %q", f)
	}
	kf := s.KeyFiles()
	if len(kf) != 3 || len(kf["smolvm/prepare.sh"]) == 0 || len(kf["smolvm/shims.sh"]) == 0 || len(kf["smolvm/sshd.sh"]) == 0 {
		t.Errorf("KeyFiles = %v", slices.Collect(func(yield func(string) bool) {
			for k := range kf {
				if !yield(k) {
					return
				}
			}
		}))
	}
}

func TestPortArgs(t *testing.T) {
	got, err := portArgs("v", backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8002}}})
	if err != nil || !slices.Equal(got, []string{"5173:5173", "8000-8002:8000-8002"}) {
		t.Errorf("ranges: %q, %v", got, err)
	}
	if got, err := portArgs("v", backend.Ports{}); err != nil || got != nil {
		t.Errorf("none: %q, %v", got, err)
	}
	if _, err := portArgs("v", backend.Ports{All: true}); names(err) != "backend_lacks backend=smolvm feature=ports-all vm=v" {
		t.Errorf("all: %v", err)
	}
	// One of smolvm's 64 is the VM's ssh.
	if _, err := portArgs("v", backend.Ports{Ranges: []backend.PortRange{{From: 3000, To: 3062}}}); err != nil {
		t.Errorf("63: %v", err)
	}
	_, err = portArgs("v", backend.Ports{Ranges: []backend.PortRange{{From: 3000, To: 3060}, {From: 4000, To: 4002}}})
	if names(err) != "smolvm_too_many_ports asked=64 max=63 vm=v" {
		t.Errorf("too many: %v", err)
	}
	// Its sshd's port is the VM's own.
	for _, r := range []backend.PortRange{{From: 22, To: 22}, {From: 20, To: 30}} {
		if _, err := portArgs("v", backend.Ports{Ranges: []backend.PortRange{r}}); names(err) != "smolvm_port_ssh port=22 vm=v" {
			t.Errorf("%v: %v", r, err)
		}
	}
	if _, err := volume("/a:b", "/a:b", true); names(err) != "smolvm_mount_colon path=/a:b" {
		t.Errorf("colon: %v", err)
	}
}

// A VM is made from its image's pack, labelled, with everything set; what
// smolvm doesn't show is recorded.
func TestCreateVM(t *testing.T) {
	s := newSandbox(t)
	s.image(t, "0123456789ab")
	spec := backend.VMSpec{
		Name: "devvm-p-0123456789abcd", Project: "/p", AgentDirs: []string{"/h/.claude"}, Tools: "/t", ToolsAt: "/mnt/devvm-tools",
		AgentForwarding: true, Size: backend.Size{CPUs: 2}, Ports: backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}}},
	}
	if err := s.CreateVM(t.Context(), "0123456789ab", spec); err != nil {
		t.Fatal(err)
	}
	want := "smolvm machine create --name devvm-p-0123456789abcd --label devvm.data=" + s.Dirs.Data +
		" --from " + s.packFile("0123456789ab") + " --cpus 2 --mem 8192 --storage 100 -v /var/lib/docker" +
		" -v /p:/p -v /h/.claude:/h/.claude:ro -v /t:/mnt/devvm-tools:ro -p 5173:5173 --ssh-agent"
	if got := s.calls(t); !slices.Equal(got, []string{want}) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	in, err := s.Inspect(t.Context(), spec.Name)
	if err != nil || !in.AgentForwarding || !slices.Equal(in.AgentDirs, spec.AgentDirs) || in.Status != backend.StatusStopped ||
		in.Size != (backend.Size{CPUs: 2, MemoryGiB: 8, DiskGiB: 100}) {
		t.Errorf("Inspect = %+v, %v", in, err)
	}
	// The same name again, or someone else's machine by it.
	if err := s.CreateVM(t.Context(), "0123456789ab", spec); err == nil {
		t.Error("made twice")
	}
	s.machine(t, "devvm-q-0123456789abcd", "created 4 8192 null false")
	s.touch(t, "smolvm-foreign-devvm-q-0123456789abcd")
	spec.Name = "devvm-q-0123456789abcd"
	if err := s.CreateVM(t.Context(), "0123456789ab", spec); names(err) != "smolvm_machine_foreign vm=devvm-q-0123456789abcd" {
		t.Errorf("foreign: %v", err)
	}
	if err := s.CreateVM(t.Context(), "ba9876543210", spec); names(err) != "image_missing key=ba9876543210" {
		t.Errorf("no image: %v", err)
	}
}

// Only devvm's machines are listed, inspected, stopped or deleted: never
// another's, whatever its name.
func TestOnlyOurs(t *testing.T) {
	s := newSandbox(t)
	s.machine(t, "devvm-a-0123456789abcd", "running 4 8192 20 true")
	s.machine(t, "devvm-img-0123456789ab", "stopped 4 8192 100 true")
	s.machine(t, ToolsVM, "stopped 4 8192 100 true")
	s.machine(t, "devvm-b-0123456789abcd", "running 4 8192 20 true")
	s.touch(t, "smolvm-foreign-devvm-b-0123456789abcd")
	ins, err := s.List(t.Context())
	if err != nil || len(ins) != 1 || ins[0].Name != "devvm-a-0123456789abcd" || ins[0].Status != backend.StatusRunning {
		t.Fatalf("List = %+v, %v", ins, err)
	}
	for _, f := range []func() error{
		func() error { _, err := s.Inspect(t.Context(), "devvm-b-0123456789abcd"); return err },
		func() error { return s.Stop(t.Context(), "devvm-b-0123456789abcd") },
		func() error { return s.Delete(t.Context(), "devvm-b-0123456789abcd") },
		func() error { return s.Start(t.Context(), "devvm-b-0123456789abcd", backend.Confinement{}) },
	} {
		if err := f(); !errors.Is(err, backend.ErrNotFound) {
			t.Errorf("someone else's: %v", err)
		}
	}
	if got := s.calls(t); len(got) != 0 {
		t.Errorf("calls: %q", got)
	}
	keys, err := s.Images(t.Context())
	if err != nil || !slices.Equal(keys, []string{"0123456789ab"}) {
		t.Errorf("Images = %q, %v", keys, err)
	}
	// SMOLVM_MACHINE_NAME is never passed on.
	if cmd := s.smolvm(t.Context(), "machine", "ls"); slices.ContainsFunc(cmd.Env, func(e string) bool {
		return strings.HasPrefix(e, "SMOLVM_MACHINE_NAME=")
	}) || !slices.Contains(cmd.Env, "SMOLVM_USAGE=0") {
		t.Errorf("env: %q", cmd.Env)
	}
}

// Start sets the network where it differs, then runs services.sh; the
// network policy is refused, and so is forwarding without an agent.
func TestStart(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-a-0123456789abcd"
	s.machine(t, vm, "stopped 4 8192 20 false")
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"smolvm machine update --name " + vm + " --net",
		"smolvm machine start --name " + vm,
		"smolvm machine exec --name " + vm + " -i -- sh -s --",
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{}); err != nil || len(s.calls(t)) != 0 {
		t.Errorf("running: %v, %q", err, s.calls(t))
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetNone}); err != nil ||
		s.calls(t)[0] != "smolvm machine update --name "+vm+" --no-net" {
		t.Errorf("no network: %v, %q", err, s.calls(t))
	}
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetProxy}); err == nil || names(err) != "not a message: smolvm: "+vm+": no decider to start it through" {
		t.Errorf("network policy without a decider: %v", err)
	}

	// Forwarding: the agent it's given, or none.
	fw := "devvm-f-0123456789abcd"
	s.machine(t, fw, "stopped 4 8192 20 true")
	if err := s.writeRecord(fw, record{Forwarding: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), fw, backend.Confinement{}); names(err) != "smolvm_no_agent vm="+fw {
		t.Errorf("no agent: %v", err)
	}
	if err := s.Start(t.Context(), fw, backend.Confinement{AgentSocket: "/a.sock"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.stub, "smolvm-agent-"+fw)); strings.TrimSpace(string(b)) != "/a.sock" {
		t.Errorf("agent: %q", b)
	}
	if got := s.Lifeline(fw); got != filepath.Join(s.stub, "smolvm-data", fw, "agent.pid") {
		t.Errorf("Lifeline = %q", got)
	}
	if err := s.Stop(t.Context(), fw); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentForwarding(t.Context(), fw, false); names(err) != "backend_lacks backend=smolvm feature=agent-toggle vm="+fw {
		t.Errorf("toggle: %v", err)
	}
	if err := s.SetAgentForwarding(t.Context(), fw, true); err != nil {
		t.Errorf("unchanged: %v", err)
	}
}

// Network policy: an allow list of the gateway, DNS answering only a name
// that doesn't exist, the proxy's address in the environment, and the start
// through devvm's decider, its token in no arguments. Confined while it
// runs on that start; a start without network policy takes it all away.
func TestStartConfined(t *testing.T) {
	s := newSandbox(t)
	t.Setenv("SMOLVM_INTERCEPTOR_TOKEN", strings.Repeat("cd", 32)) // the user's own: never passed on
	vm := "devvm-n-0123456789abcd"
	s.machine(t, vm, "stopped 4 8192 20 false")
	tok := strings.Repeat("ab", 32)
	c := backend.Confinement{
		Net: backend.NetProxy, ProxyPort: 3128, ProxyUser: "devvm", ProxyPassword: "pw", Env: map[string]string{"NO_PROXY": "localhost"},
		Decider: "127.0.0.1:4000", DeciderToken: tok,
	}
	for _, bad := range []backend.Confinement{
		{Net: backend.NetProxy, Decider: "10.0.0.1:4000", DeciderToken: tok},
		{Net: backend.NetProxy, Decider: "127.0.0.1:4000", DeciderToken: "ab"},
	} {
		if err := s.Start(t.Context(), vm, bad); err == nil {
			t.Errorf("%+v: started", bad)
		}
	}
	// Made by an older devvm: the proxy's variables in its environment go.
	if err := s.writeRecord(vm, record{Env: map[string]string{"HTTPS_PROXY": "http://devvm:old@100.96.0.1:1"}}); err != nil { //nolint:gosec // the test's own password
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	f := s.proxyFile(vm)
	secrets := " --secret-file HTTP_PROXY=" + f + " --secret-file HTTPS_PROXY=" + f + " --secret-file http_proxy=" + f + " --secret-file https_proxy=" + f
	want := []string{
		"smolvm machine update --name " + vm + " --reset-allow-list --allow-cidr 100.96.0.1/32 --allow-host-pattern devvm.invalid",
		"smolvm machine update --name " + vm + " --remove-env HTTPS_PROXY -e NO_PROXY=localhost",
		"smolvm machine start --name " + vm + " --mediated-egress --egress-interceptor 127.0.0.1:4000",
		"smolvm machine exec --name " + vm + secrets + " -i -- sh -s --",
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	// The proxy's URL, password and all, is in its file alone (mode 600),
	// which each exec names.
	if b, err := os.ReadFile(f); err != nil || string(b) != "http://devvm:pw@100.96.0.1:3128" { //nolint:gosec // the test's own password
		t.Errorf("proxy file: %q, %v", b, err)
	}
	if fi, err := os.Stat(f); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("proxy file: %v, %v", fi, err)
	}
	if args, err := s.userArgs(vm, "/", false, false, []string{"true"}); err != nil || !strings.Contains(strings.Join(args, " "), secrets[1:]+" -u u ") {
		t.Errorf("exec: %q, %v", args, err)
	}
	if strings.Contains(strings.Join(s.calls(t), " "), ":pw@") {
		t.Error("the password is in smolvm's arguments")
	}
	if b, _ := os.ReadFile(filepath.Join(s.stub, "smolvm-decider-"+vm)); strings.TrimSpace(string(b)) != "127.0.0.1:4000 "+tok {
		t.Errorf("decider: %q", b)
	}
	if strings.Contains(strings.Join(s.calls(t), " "), tok) {
		t.Error("the token is in smolvm's arguments")
	}
	if ok, err := s.Confined(t.Context(), vm); !ok || err != nil {
		t.Errorf("Confined = %v, %v", ok, err)
	}
	// Its allow list widened while it runs: no longer confined, and the next
	// start sets it again.
	if out, err := exec.CommandContext(t.Context(), "smolvm", "machine", "update", "--name", vm, "--allow-cidr", "1.1.1.1/32").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if ok, err := s.Confined(t.Context(), vm); ok || err != nil {
		t.Errorf("widened: Confined = %v, %v", ok, err)
	}
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if got := s.calls(t); len(got) == 0 || got[0] != want[0] {
		t.Errorf("widened, then started: %q", got)
	}
	if ok, err := s.Confined(t.Context(), vm); !ok || err != nil {
		t.Errorf("started again: Confined = %v, %v", ok, err)
	}
	// Started again as it was: nothing to change.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.Confined(t.Context(), vm); ok {
		t.Error("stopped, yet confined")
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, c); err != nil {
		t.Fatal(err)
	}
	if got := s.calls(t); len(got) != 2 || !strings.HasPrefix(got[0], "smolvm machine start ") {
		t.Errorf("started again: %q", got)
	}
	// Started some other way since: not confined.
	pid := filepath.Join(s.stub, "smolvm-data", vm, "agent.pid")
	if err := os.WriteFile(pid, []byte(strconv.Itoa(os.Getpid())+"\n1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Confined(t.Context(), vm); ok || err != nil {
		t.Errorf("another start: Confined = %v, %v", ok, err)
	}
	// Without network policy: smolvm's own network, nothing of the decider's.
	if err := s.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	s.reset(t)
	if err := s.Start(t.Context(), vm, backend.Confinement{Net: backend.NetDirect}); err != nil {
		t.Fatal(err)
	}
	want = []string{
		"smolvm machine update --name " + vm + " --net --reset-allow-list --no-egress-interceptor",
		"smolvm machine update --name " + vm + " --remove-env NO_PROXY",
		"smolvm machine start --name " + vm,
		"smolvm machine exec --name " + vm + " -i -- sh -s --",
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	if _, err := os.Stat(f); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("proxy file: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(s.stub, "smolvm-decider-"+vm)); strings.TrimSpace(string(b)) != "none unset" {
		t.Errorf("decider: %q", b)
	}
	if ok, _ := s.Confined(t.Context(), vm); ok {
		t.Error("confined without network policy")
	}
}

// Base: a machine from Ubuntu's container image by digest, prepared, its
// scripts run between the stand-ins, packed, then deleted.
func TestBuildBase(t *testing.T) {
	s := newSandbox(t)
	script := filepath.Join(t.TempDir(), "base.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\necho base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	built, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: "0123456789ab", Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "base.sh", Path: script}},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("ab", 32)
	if built.Libc != "gnu" || built.Origin != UbuntuImage+"@"+digest {
		t.Errorf("Built = %+v", built)
	}
	inst := "devvm-img-0123456789ab"
	got := s.calls(t)
	want := []string{
		"smolvm machine create --name " + inst + " --label devvm.data=" + s.Dirs.Data +
			" --cpus 4 --mem 8192 --storage 100 -v /var/lib/docker --net --image docker.io/library/ubuntu@" + digest,
		"smolvm machine start --name " + inst,
		"smolvm machine exec --name " + inst + " -i -- sh -s --",
		"smolvm machine exec --name " + inst + " -i -- sh -s -- u 501",
		"smolvm machine exec --name " + inst + " -- sh -c if command -v apk >/dev/null; then echo musl; else echo gnu; fi",
		"smolvm machine exec --name " + inst + " -i -- sh -s -- on",
		"smolvm machine exec --name " + inst + " -i -u u -w / -e USER=u -e LOGNAME=u -e HOME=/home/u.guest -e SHELL=/bin/bash -- sh -c " +
			loginEnv + " sh bash -s",
		"smolvm machine exec --name " + inst + " -i -- sh -s --",
		"smolvm machine exec --name " + inst + " -i -- sh -s -- off",
		"smolvm machine stop --name " + inst,
		"smolvm pack create --from-vm " + inst + " -o " + s.Dirs.SmolvmImage("0123456789ab") + ".tmp/image",
		"smolvm machine delete --name " + inst + " -f",
	}
	if !slices.Equal(got, want) {
		t.Errorf("calls:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if _, err := os.Stat(s.packFile("0123456789ab")); err != nil {
		t.Errorf("no pack: %v", err)
	}
	if keys, err := s.Images(t.Context()); err != nil || !slices.Equal(keys, []string{"0123456789ab"}) {
		t.Errorf("Images = %q, %v", keys, err)
	}
	// A VM made from it keeps it.
	if err := s.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{Name: "devvm-p-0123456789abcd"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteImage(t.Context(), "0123456789ab"); names(err) != "image_in_use key=0123456789ab vms=devvm-p-0123456789abcd" {
		t.Errorf("in use: %v", err)
	}
	if err := s.Delete(t.Context(), "devvm-p-0123456789abcd"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteImage(t.Context(), "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Dirs.SmolvmImage("0123456789ab")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("still there: %v", err)
	}
}

// A failed script keeps the build machine, stopped; an image only Docker
// has is refused.
func TestBuildFails(t *testing.T) {
	s := newSandbox(t)
	s.image(t, "0123456789ab")
	s.touch(t, "script-fails")
	script := filepath.Join(t.TempDir(), "foo.sh")
	if err := os.WriteFile(script, []byte("#!/bin/bash\nexit 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "foo", Key: "ba9876543210", Parent: "0123456789ab", Net: backend.NetInternet,
		Scripts: []backend.Script{{Name: "foo.sh", Path: script}},
	})
	if names(err) != "smolvm_script_failed script=foo.sh vm=devvm-img-ba9876543210" {
		t.Errorf("failed: %v", err)
	}
	m, err := s.inspect(t.Context(), "devvm-img-ba9876543210")
	if err != nil || m.status() != backend.StatusStopped {
		t.Errorf("kept: %+v, %v", m, err)
	}
	if keys, _ := s.Images(t.Context()); !slices.Contains(keys, "ba9876543210") {
		t.Errorf("unfinished not listed: %q", keys)
	}
	_, err = s.BuildImage(t.Context(), backend.ImageSpec{
		Image: "base", Key: "aaaaaaaaaaaa", Net: backend.NetInternet, From: &backend.From{Ref: "mine:1", Digest: "sha256:" + strings.Repeat("cd", 32)},
	})
	if names(err) != "smolvm_from_registry_only image=mine:1" {
		t.Errorf("Docker's: %v", err)
	}
}

// Sizes change where they differ; disks only grow; ports and agent folders
// replace what was set.
func TestSet(t *testing.T) {
	s := newSandbox(t)
	vm := "devvm-a-0123456789abcd"
	s.machine(t, vm, "stopped 4 8192 100 true")
	if err := s.writeRecord(vm, record{AgentDirs: []string{"/h/.claude"}, Ports: []string{"3000:3000"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSize(t.Context(), vm, backend.Size{CPUs: 4, MemoryGiB: 4, DiskGiB: 120}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSize(t.Context(), vm, backend.Size{DiskGiB: 50}); names(err) != "disk_shrink have=120 vm="+vm+" want=50" {
		t.Errorf("shrink: %v", err)
	}
	if err := s.SetPorts(t.Context(), vm, backend.Ports{Ranges: []backend.PortRange{{From: 8000, To: 8000}}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentDirs(t.Context(), vm, []string{"/h/.codex"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"smolvm machine update --name " + vm + " --mem 4096 --storage 120",
		"smolvm machine update --name " + vm + " --remove-port 3000:3000 -p 8000:8000",
		"smolvm machine update --name " + vm + " --remove-volume /h/.claude:/h/.claude -v /h/.codex:/h/.codex:ro",
	}
	if got := s.calls(t); !slices.Equal(got, want) {
		t.Errorf("calls:\n%q\nwant\n%q", got, want)
	}
	s.machine(t, vm, "running 4 4096 120 true")
	if err := s.SetSize(t.Context(), vm, backend.Size{CPUs: 2}); names(err) != "vm_not_stopped vm="+vm {
		t.Errorf("running: %v", err)
	}
}

// On Linux, smolvm's state is devvm's own.
func TestLinuxStateDir(t *testing.T) {
	s := newSandbox(t)
	s.OS = "linux"
	cmd := s.smolvm(t.Context(), "machine", "ls", "--json")
	if want := []string{"smolvm", "machine", "ls", "--state-dir", filepath.Join(s.Dirs.Smolvm(), "state"), "--json"}; !slices.Equal(cmd.Args, want) {
		t.Errorf("args = %q", cmd.Args)
	}
	if cmd := s.smolvm(t.Context(), "--version"); len(cmd.Args) != 2 {
		t.Errorf("--version: %q", cmd.Args)
	}
}

func TestProblems(t *testing.T) {
	s := newSandbox(t)
	if ps := s.Problems(t.Context()); len(ps) != 0 {
		t.Errorf("all there: %+v", ps)
	}
	if err := os.WriteFile(filepath.Join(s.stub, "smolvm-version"), []byte("1.31.0"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Check(t.Context()); !strings.HasPrefix(names(err), "smolvm_old") {
		t.Errorf("old: %v", err)
	}
	s.OS, s.Arch = "darwin", "amd64"
	if err := s.Check(t.Context()); names(err) != "smolvm_host_unsupported host=darwin/amd64" {
		t.Errorf("Intel Mac: %v", err)
	}
	for _, v := range []struct {
		v  string
		ok bool
	}{{"1.32.2", true}, {"1.40.0", true}, {"2.0.0", true}, {"1.9.9", false}, {"x", false}} {
		if recent(v.v) != v.ok {
			t.Errorf("recent(%s)", v.v)
		}
	}
}
