package lima

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/stubtest"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// testUI prints nothing, runs commands as they are, counts no steps and
// keeps no cleanups.
type testUI struct{}

func (testUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }
func (testUI) Info(msg.M)                                                {}
func (testUI) Note(msg.M)                                                {}
func (testUI) Warn(msg.M)                                                {}
func (testUI) LogNote()                                                  {}
func (testUI) WithSteps(ctx context.Context, _ int) context.Context      { return ctx }
func (testUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (testUI) Interrupted() bool                                         { return false }

// call is a command the fake runner was given.
type call struct {
	argv  []string // the program's base name, then its arguments
	env   []string
	dir   string
	stdin string // what a script fed on its input got
}

func (c call) line() string { return strings.Join(c.argv, " ") }

// fakeRunner runs nothing: it records each command and answers Output from
// out, by command line ("limactl list --json"). Commands whose line starts
// with a key of fail fail.
type fakeRunner struct {
	mu    sync.Mutex
	calls []call
	out   map[string]string
	fail  []string
	do    func(c call) // run for each command
}

func (f *fakeRunner) record(cmd *exec.Cmd) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	argv := append([]string{filepath.Base(cmd.Path)}, cmd.Args[1:]...)
	c := call{argv: argv, env: cmd.Env, dir: cmd.Dir}
	if r, ok := cmd.Stdin.(*bytes.Reader); ok {
		b, _ := io.ReadAll(r)
		c.stdin = string(b)
	}
	f.calls = append(f.calls, c)
	if f.do != nil {
		f.do(c)
	}
	for _, p := range f.fail {
		if strings.HasPrefix(c.line(), p) {
			return errors.New("failed")
		}
	}
	return nil
}

func (f *fakeRunner) Run(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return f.record(cmd) }

func (f *fakeRunner) Output(cmd *exec.Cmd) ([]byte, error) {
	if err := f.record(cmd); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return []byte(f.out[f.calls[len(f.calls)-1].line()]), nil
}

func (f *fakeRunner) Attach(cmd *exec.Cmd) error { return f.record(cmd) }

// lines returns the calls' command lines, without the read-only ones
// (list, --version, qemu-img info).
func (f *fakeRunner) lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ls []string
	for _, c := range f.calls {
		if l := c.line(); !strings.HasPrefix(l, "limactl list") && !strings.HasPrefix(l, "qemu-img info") && l != "limactl --version" {
			ls = append(ls, l)
		}
	}
	return ls
}

// sandbox points devvm's folders into a new folder under /tmp (short, for
// Lima's socket paths), with the stubs first on PATH.
func sandbox(t *testing.T) (paths.Dirs, *stubtest.Stubs) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "dvl.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"home", "data", "config"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stubs := stubtest.New(t)
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("DEVVM_LIMA_HOME", filepath.Join(root, "lima"))
	t.Setenv("LIMA_HOME", "/elsewhere") // never devvm's
	t.Setenv("DEVVM_MESSAGES", "names")
	d, err := paths.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return d, stubs
}

// testLima returns the backend on a sandbox, as on host os, with the fake
// runner; warnings go to warned.
func testLima(t *testing.T, goos string) (*Lima, *fakeRunner, *msg.Recorder) {
	t.Helper()
	l, f, warned, _ := limaOn(t, goos)
	return l, f, warned
}

// limaOn is testLima, with the sandbox's stubs.
func limaOn(t *testing.T, goos string) (*Lima, *fakeRunner, *msg.Recorder, *stubtest.Stubs) {
	t.Helper()
	d, stubs := sandbox(t)
	l := New(d, assets.FS, testUI{}, platform.Host{}, "")
	f := &fakeRunner{out: map[string]string{}}
	warned := &msg.Recorder{}
	l.Runner, l.OS, l.Arch, l.Self = f, goos, "arm64", "/opt/my devvm/devvm"
	l.Getenv = func(string) string { return "" }
	l.Spawn = func(string, []string, *os.File) (*os.Process, error) { return nil, errors.New("no build proxy here") }
	l.Tick = time.Millisecond
	l.MacOS = func() string { return "26.0" }
	l.KVM = filepath.Join(t.TempDir(), "kvm")
	if err := os.WriteFile(l.KVM, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	l.Warn = warned.Add
	return l, f, warned, stubs
}

// stubLima is testLima with the real runner, so limactl is the stub.
func stubLima(t *testing.T) (*Lima, *msg.Recorder, *stubtest.Stubs) {
	t.Helper()
	l, _, warned, stubs := limaOn(t, "darwin")
	l.Runner = stubtest.ExecRunner{}
	return l, warned, stubs
}

// An image's instance is devvm-img-<key>; a project VM's from a folder
// named img-… isn't one, so gc never takes it for an image.
func TestImageKey(t *testing.T) {
	for _, c := range []struct {
		name, key string
		ok        bool
	}{
		{"devvm-img-0123456789ab", "0123456789ab", true},
		{"devvm-img-x", "x", true},
		{"devvm-img-keep-0123456789abcd", "keep-0123456789abcd", false},
		{"devvm-img-", "", false},
		{"devvm-web-0123456789abcd", "", false},
		{"other", "", false},
	} {
		if key, ok := imageKey(c.name); ok != c.ok || ok && key != c.key {
			t.Errorf("imageKey(%q) = %q, %v", c.name, key, ok)
		}
	}
}

// Lima runs on devvm's own Lima home (DEVVM_LIMA_HOME, never LIMA_HOME),
// from devvm's data folder, never the caller's: Lima writes
// lima.REJECTED.yaml into the folder it runs in.
// On Linux hosts an image's and a tools build's disks start as full
// copies of their parent's, so those floors are higher; a VM's is an
// overlay, so its floor is the same.
func TestFloors(t *testing.T) {
	mac, _, _ := testLima(t, "darwin")
	linux, _, _ := testLima(t, "linux")
	m, l := mac.Floors(), linux.Floors()
	if l.Image <= m.Image || l.Tools <= m.Tools || l.VM != m.VM || l.Base != m.Base || l.PerTool != m.PerTool {
		t.Errorf("macOS %+v, Linux %+v", m, l)
	}
}

func TestLimactlUsesDevvmsHomeAndFolder(t *testing.T) {
	l, _, stubs := stubLima(t)
	t.Chdir(t.TempDir())
	if _, err := l.List(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := "cwd=" + l.Dirs.Data + " LIMA_HOME=" + os.Getenv("DEVVM_LIMA_HOME") + "\n"
	if got := stubs.Read(t, "limactl-env"); got != want {
		t.Errorf("limactl ran with %q, want %q", got, want)
	}
}

func TestSetEnv(t *testing.T) {
	got := setEnv([]string{"A=1", "B=2", "AB=3"}, "A=9", "B", "C=x=y")
	if want := []string{"AB=3", "A=9", "C=x=y"}; !slices.Equal(got, want) {
		t.Errorf("setEnv = %q, want %q", got, want)
	}
}

// Lima's global configs in devvm's Lima home would reach every VM, and a
// network devvm didn't give a VM would reach around its proxy: no start.
func TestStartRefusesGlobalConfigsAndNetworks(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"devvm-img-x","status":"Stopped","config":{"networks":[{"vzNAT":true}]}}`
	cfg := filepath.Join(l.Dirs.Lima, "_config")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "default.yaml"), []byte("networks: [{vzNAT: true}]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := l.Start(t.Context(), "devvm-img-x", backend.Confinement{Net: backend.NetInternet})
	if msg.IDOf(err) != "lima_global_config" || !strings.Contains(err.Error(), "files="+filepath.Join(cfg, "default.yaml")) {
		t.Errorf("with default.yaml: %v", err)
	}
	f.out["limactl --version"] = "limactl version 2.2.1\n"
	if err := l.Check(t.Context()); msg.IDOf(err) != "lima_global_config" {
		t.Errorf("Check with default.yaml: %v", err)
	}
	if err := os.Remove(filepath.Join(cfg, "default.yaml")); err != nil {
		t.Fatal(err)
	}
	err = l.Start(t.Context(), "devvm-img-x", backend.Confinement{Net: backend.NetInternet})
	if want := "vm_has_networks file=" + filepath.Join(l.Dirs.Lima, "devvm-img-x", "lima.yaml") + " vm=devvm-img-x"; err == nil || err.Error() != want {
		t.Errorf("with a network: %v, want %s", err, want)
	}
	if got := f.lines(); len(got) > 0 {
		t.Errorf("ran %q", got)
	}
}

// Lima keeps a VM's ssh socket in its folder, OpenSSH adds 17 characters
// while making it, and a socket path has at most 104: the name gets what's
// left.
func TestNameMax(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	n := l.NameMax()
	if len(l.Dirs.Lima)+1+n+len("/ssh.sock")+17 != 104 {
		t.Errorf("NameMax %d for a Lima home of %d characters", n, len(l.Dirs.Lima))
	}
	long := "devvm-" + strings.Repeat("x", n) + "-0123456789abcd"
	err := l.CreateVM(t.Context(), "k", backend.VMSpec{Name: long})
	if msg.IDOf(err) != "lima_home_too_long" {
		t.Errorf("a name too long: %v", err)
	}
	if len(f.calls) > 0 {
		t.Errorf("ran %q", f.lines())
	}
	l.Dirs.Lima = "/" + strings.Repeat("x", 80)
	if l.NameMax() != 0 {
		t.Errorf("NameMax for a long home: %d", l.NameMax())
	}
}

// Everything is native but ports, which devvm's forwarder forwards: an
// emulation, reviewed.
func TestCaps(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	for _, f := range backend.Features {
		c := l.Caps()[f]
		if f == backend.FeaturePorts || f == backend.FeaturePortsAll {
			if c != backend.Emulated(true) {
				t.Errorf("%s: %+v, want emulated and reviewed", f, c)
			}
		} else if !c.IsNative() {
			t.Errorf("%s isn't native", f)
		}
	}
}

// base's key hashes the pinned Ubuntu cloud image for the host: another
// pin is another key.
func TestKeyFilesPinUbuntu(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	keys := map[string]bool{}
	for _, arch := range []string{"arm64", "amd64", ""} {
		in := image.Inputs{Backend: Name, Assets: assets.FS}
		if arch != "" {
			l.Arch = arch
			kf := l.KeyFiles()
			if got, want := string(kf["ubuntu-image"]), ubuntu.CloudPin(arch); len(kf) != 1 || got != want {
				t.Errorf("%s: KeyFiles = %q, want ubuntu-image %q", arch, kf, want)
			}
			in.BackendFiles = kf
		}
		key, err := in.Key(image.Base)
		if err != nil {
			t.Fatal(err)
		}
		keys[key] = true
	}
	if len(keys) != 3 {
		t.Errorf("base's keys: %v, want one per pin and one without", keys)
	}
}

const listed = `{"name":"devvm-img-0123456789ab","status":"Stopped","cpus":4,"memory":8589934592,"disk":107374182400}
{"name":"devvm-img-x-0123456789abcd","status":"Running","cpus":2,"memory":4294967296,"disk":107374182400,"config":{"mounts":[{"location":"/p","writable":true},{"location":"/h/.claude","mountPoint":"/h/.claude","writable":false},{"location":"/d/tools","mountPoint":"/mnt/devvm-tools","writable":false}],"ssh":{"forwardAgent":true}}}
{"name":"devvm-tools-build","status":"Stopped"}
`

// Images are devvm-img-<key>; a project VM for a folder named img-… is
// still a project VM.
func TestListAndImages(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = listed
	keys, err := l.Images(t.Context())
	if err != nil || !slices.Equal(keys, []string{"0123456789ab"}) {
		t.Errorf("Images: %q, %v", keys, err)
	}
	ins, err := l.List(t.Context())
	if err != nil || len(ins) != 2 || ins[0].Name != "devvm-img-x-0123456789abcd" || ins[1].Name != "devvm-tools-build" {
		t.Fatalf("List: %+v, %v", ins, err)
	}
	want := backend.Instance{
		Name: "devvm-img-x-0123456789abcd", Status: backend.StatusRunning,
		Size:      backend.Size{CPUs: 2, MemoryGiB: 4, DiskGiB: 100},
		AgentDirs: []string{"/h/.claude"}, AgentForwarding: true,
	}
	if got := ins[0]; got.Name != want.Name || got.Status != want.Status || got.Size != want.Size ||
		!slices.Equal(got.AgentDirs, want.AgentDirs) || got.AgentForwarding != want.AgentForwarding {
		t.Errorf("Inspect: %+v, want %+v", got, want)
	}
	if _, err := l.Inspect(t.Context(), "nope"); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("Inspect nope: %v", err)
	}
	if err := l.Stop(t.Context(), "nope"); !errors.Is(err, backend.ErrNotFound) {
		t.Errorf("Stop nope: %v", err)
	}
}

func TestDeleteImage(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = listed
	if err := l.DeleteImage(t.Context(), "x-0123456789abcd"); msg.IDOf(err) != "not_an_image" {
		t.Errorf("a project VM: %v", err)
	}
	if err := l.DeleteImage(t.Context(), "missing0000"); err != nil {
		t.Errorf("a missing image: %v", err)
	}
	if err := l.DeleteImage(t.Context(), "0123456789ab"); err != nil {
		t.Fatal(err)
	}
	if got, want := f.lines(), []string{"limactl delete -f devvm-img-0123456789ab"}; !slices.Equal(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}

func TestImageSize(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	if _, err := l.ImageSize(t.Context(), "x-0123456789abcd"); msg.IDOf(err) != "not_an_image" {
		t.Errorf("a project VM: %v", err)
	}
	if _, err := l.ImageSize(t.Context(), "0123456789ab"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing image: %v", err)
	}
	d := l.Dirs.LimaVM("devvm-img-0123456789ab")
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.disk("devvm-img-0123456789ab"), make([]byte, 64<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := l.ImageSize(t.Context(), "0123456789ab"); err != nil || n < 64<<10 {
		t.Errorf("ImageSize = %d, %v; want at least its disk's 64 KiB", n, err)
	}
}

// Linux hosts: an image whose disk project VMs are overlays of is never
// deleted. A running VM's QEMU holds its disk's write lock, which refuses
// qemu-img info unless it shares the disk (-U).
func TestDeleteImageInUse(t *testing.T) {
	l, f, _ := testLima(t, "linux")
	f.out["limactl list --json"] = listed
	f.fail = []string{"qemu-img info --output=json"}
	vm := "devvm-img-x-0123456789abcd"
	for _, d := range []string{"devvm-img-0123456789ab", vm} {
		if err := os.MkdirAll(l.Dirs.LimaVM(d), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.disk(d), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.out["qemu-img info -U --output=json "+l.disk(vm)] = `{"format":"qcow2","backing-filename":"` + l.disk("devvm-img-0123456789ab") + `"}`
	err := l.DeleteImage(t.Context(), "0123456789ab")
	if err == nil || err.Error() != "image_in_use key=0123456789ab vms="+vm {
		t.Errorf("DeleteImage: %v", err)
	}
	if got := f.lines(); len(got) > 0 {
		t.Errorf("ran %q", got)
	}
}

func TestCreateVM(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"devvm-img-0123456789ab","status":"Running","cpus":4,"memory":8589934592,"disk":107374182400}
{"name":"devvm-p-0123456789abcd","status":"Stopped","cpus":4,"memory":8589934592,"disk":107374182400}`
	err := l.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{
		Name: "devvm-p-0123456789abcd", Project: "/my project", AgentDirs: []string{"/h/.claude"},
		Tools: "/d/tools", ToolsAt: "/mnt/devvm-tools", AgentForwarding: true,
		Size:  backend.Size{CPUs: 6, MemoryGiB: 8, DiskGiB: 150},
		Ports: backend.Ports{Ranges: []backend.PortRange{{From: 5173, To: 5173}}}, SSHPort: 23456,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"limactl stop devvm-img-0123456789ab",
		`limactl clone --tty=false devvm-img-0123456789ab devvm-p-0123456789abcd --mount-none` +
			` --set .mounts = [{"location":"/my project","writable":true},{"location":"/h/.claude","writable":false},{"location":"/d/tools","mountPoint":"/mnt/devvm-tools","writable":false}]` +
			` --set .ssh.forwardAgent=true --set .ssh.localPort=23456 --set ` + noForwardsSet(),
		"limactl edit --tty=false devvm-p-0123456789abcd --cpus 6 --disk 150",
	}
	if got := f.lines(); !slices.Equal(got, want) {
		t.Errorf("ran\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Its ports are its record's, for devvm's forwarder.
	if p, err := l.Forwards.read("devvm-p-0123456789abcd"); err != nil || p.All || !slices.Equal(p.Ranges, []backend.PortRange{{From: 5173, To: 5173}}) {
		t.Errorf("ports: %+v, %v", p, err)
	}
	err = l.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{Name: "devvm-q-0123456789abcd", Project: "/a,b"})
	if err == nil || err.Error() != "mount_comma path=/a,b" {
		t.Errorf("a comma: %v", err)
	}
	if err := l.CreateVM(t.Context(), "fedcba987654", backend.VMSpec{Name: "devvm-q-0123456789abcd"}); msg.IDOf(err) != "image_missing" {
		t.Errorf("no image: %v", err)
	}
	f.fail = []string{"limactl clone"}
	err = l.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{Name: "devvm-q-0123456789abcd"})
	if le, ok := errors.AsType[*backend.LoggedError](err); !ok || le.Msg.ID() != "vm_create_failed" {
		t.Errorf("a failed clone: %v", err)
	}
}

// Linux hosts: a project VM's disk becomes an overlay of its image's,
// owner-only. macOS has none: APFS clones already share.
func TestOverlay(t *testing.T) {
	for _, host := range []string{"linux", "darwin"} {
		l, f, _ := testLima(t, host)
		img, vm := "devvm-img-0123456789ab", "devvm-p-0123456789abcd"
		f.out["limactl list --json"] = `{"name":"` + img + `","status":"Stopped"}` + "\n" + `{"name":"` + vm + `","status":"Stopped"}`
		for _, d := range []string{img, vm} {
			if err := os.MkdirAll(l.Dirs.LimaVM(d), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(l.disk(d), []byte("disk"), 0o644); err != nil { //nolint:gosec // a test's
				t.Fatal(err)
			}
		}
		f.out["qemu-img info -U --output=json "+l.disk(vm)] = `{"format":"qcow2"}`
		f.do = func(c call) {
			if c.argv[0] == "qemu-img" && c.argv[1] == "create" {
				if err := os.WriteFile(c.argv[len(c.argv)-1], []byte("overlay"), 0o644); err != nil { //nolint:gosec // as qemu-img would
					t.Error(err)
				}
			}
		}
		if err := l.CreateVM(t.Context(), "0123456789ab", backend.VMSpec{Name: vm}); err != nil {
			t.Fatal(err)
		}
		created := slices.Contains(f.lines(), "qemu-img create -q -f qcow2 -b "+l.disk(img)+" -F qcow2 "+l.disk(vm))
		if host == "linux" {
			if !created {
				t.Errorf("no overlay: %q", f.lines())
			}
			// The copy is gone; the overlay may hold tokens: owner only.
			b, err := os.ReadFile(l.disk(vm))
			if err != nil || string(b) != "overlay" {
				t.Errorf("the VM's disk: %q, %v", b, err)
			}
			if fi, err := os.Stat(l.disk(vm)); err != nil || fi.Mode().Perm() != 0o600 {
				t.Errorf("the overlay's mode: %v, %v", fi.Mode(), err)
			}
		} else if created || slices.ContainsFunc(f.calls, func(c call) bool { return c.argv[0] == "qemu-img" }) {
			t.Errorf("macOS ran qemu-img: %q", f.lines())
		}
	}
}

// The Set methods change a stopped VM, only what differs; a running one is
// refused (it applies at its next start), and so is a smaller disk.
func TestSetSize(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"v","status":"Stopped","cpus":4,"memory":8589934592,"disk":107374182400}
{"name":"r","status":"Running","cpus":4,"memory":8589934592,"disk":107374182400}`
	for _, tc := range []struct {
		vm   string
		size backend.Size
		want string // the edit, or the error
	}{
		{"v", backend.Size{CPUs: 6, MemoryGiB: 8, DiskGiB: 150}, "limactl edit --tty=false v --cpus 6 --disk 150"},
		{"v", backend.Size{MemoryGiB: 3.5}, "limactl edit --tty=false v --memory 3.5"},
		{"v", backend.Size{CPUs: 4, DiskGiB: 100}, ""},
		{"v", backend.Size{DiskGiB: 50}, "disk_shrink have=100 vm=v want=50"},
		{"r", backend.Size{CPUs: 6}, "vm_not_stopped vm=r"},
	} {
		f.calls = nil
		err := l.SetSize(t.Context(), tc.vm, tc.size)
		got := strings.Join(f.lines(), "\n")
		if err != nil {
			got = err.Error()
		}
		if got != tc.want {
			t.Errorf("SetSize(%s, %+v): %q, want %q", tc.vm, tc.size, got, tc.want)
		}
	}
}

// Lima forwards nothing: every guest port, TCP or UDP, is ignored on
// 127.0.0.1 (::1 too), 0.0.0.0 and ::, before Lima's own rule.
func TestNoForwardsSet(t *testing.T) {
	want := `.portForwards = [{"guestIP":"127.0.0.1","guestPortRange":[1,65535],"proto":"any","ignore":true},` +
		`{"guestIP":"0.0.0.0","guestPortRange":[1,65535],"proto":"any","ignore":true},` +
		`{"guestIP":"::","guestPortRange":[1,65535],"proto":"any","ignore":true}]`
	if got := noForwardsSet(); got != want {
		t.Errorf("noForwardsSet() =\n%s\nwant\n%s", got, want)
	}
}

// A VM's ports record: all, none or a list; none forwarded without one.
func TestPortsRecord(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	f := l.Forwards
	if p, err := f.read("v"); err == nil {
		t.Errorf("no record: %+v, want an error (none forwarded)", p)
	}
	for _, p := range []backend.Ports{
		{}, {All: true}, {Ranges: []backend.PortRange{{From: 5173, To: 5173}, {From: 8000, To: 8010}}},
	} {
		if err := f.write("v", p); err != nil {
			t.Fatal(err)
		}
		got, err := f.read("v")
		if err != nil || got.All != p.All || !slices.Equal(got.Ranges, p.Ranges) {
			t.Errorf("%+v read as %+v, %v", p, got, err)
		}
	}
	if err := os.WriteFile(f.file("v"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.read("v"); err == nil {
		t.Error("a bad record read")
	}
}

func TestSetPortsAgentDirsAndForwarding(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	f.out["limactl list --json"] = `{"name":"v","status":"Stopped","config":{"mounts":[{"location":"/p","writable":true},{"location":"/h/.claude","mountPoint":"/h/.claude"},{"location":"/d/tools","mountPoint":"/mnt/devvm-tools"}],"ssh":{"forwardAgent":false}}}
{"name":"r","status":"Running"}`
	ctx := t.Context()
	steps := []struct {
		do   func() error
		want string
	}{
		{func() error { return l.SetPorts(ctx, "v", backend.Ports{All: true}, backend.Confinement{}) }, ""},
		{func() error { return l.SetAgentDirs(ctx, "v", []string{"/h/.claude"}) }, ""},
		{
			func() error { return l.SetAgentDirs(ctx, "v", []string{"/h/.codex"}) },
			`limactl edit --tty=false v --set .mounts = [{"location":"/p","writable":true},{"location":"/d/tools","mountPoint":"/mnt/devvm-tools"},{"location":"/h/.codex","writable":false}]`,
		},
		{func() error { return l.SetAgentDirs(ctx, "v", []string{"/h/a,b"}) }, "mount_comma path=/h/a,b"},
		{func() error { return l.SetAgentForwarding(ctx, "v", false) }, ""},
		{func() error { return l.SetAgentForwarding(ctx, "v", true) }, "limactl edit --tty=false v --set .ssh.forwardAgent=true"},
		// A running VM's: no edit (its forwarder, ports_test.go).
		{func() error { return l.SetPorts(ctx, "r", backend.Ports{}, backend.Confinement{}) }, ""},
		{func() error { return l.SetAgentDirs(ctx, "r", nil) }, "vm_not_stopped vm=r"},
		{func() error { return l.SetAgentForwarding(ctx, "r", true) }, "vm_not_stopped vm=r"},
	}
	for i, s := range steps {
		f.calls = nil
		err := s.do()
		got := strings.Join(f.lines(), "\n")
		if err != nil {
			got = err.Error()
		}
		if got != s.want {
			t.Errorf("step %d: %q, want %q", i, got, s.want)
		}
	}
}

func TestSSH(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	cfg := filepath.Join(t.TempDir(), "ssh.config")
	// Lima's own: host keys unchecked, a shared connection, and (here) a
	// command to run. Only the login options are kept.
	content := "Host lima-v\n  IdentityFile \"/h/.lima/_config/user\"\n  StrictHostKeyChecking no\n  UserKnownHostsFile /dev/null\n" +
		"  NoHostAuthenticationForLocalhost yes\n  PreferredAuthentications publickey\n  Compression no\n  BatchMode yes\n  IdentitiesOnly yes\n" +
		"  ControlMaster auto\n  ControlPath \"/h/.lima/v/ssh.sock\"\n  ControlPersist yes\n  PermitLocalCommand yes\n  LocalCommand touch /x/ran\n" +
		"  User cj\n  Hostname 127.0.0.1\n  Port 41083\n"
	if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f.out["limactl list --json"] = `{"name":"v","status":"Running","sshLocalPort":41083,"sshConfigFile":"` + cfg + `"}
{"name":"s","status":"Stopped"}`
	got, err := l.SSH(t.Context(), "v")
	want := backend.SSHTarget{Host: "127.0.0.1", Port: 41083, User: "cj", ConfigFile: cfg, Options: []backend.SSHOption{
		{Key: "IdentityFile", Value: `"/h/.lima/_config/user"`},
		{Key: "PreferredAuthentications", Value: "publickey"},
		{Key: "Compression", Value: "no"},
		{Key: "BatchMode", Value: "yes"},
		{Key: "IdentitiesOnly", Value: "yes"},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("SSH: %+v, %v; want %+v", got, err, want)
	}
	if _, err := l.SSH(t.Context(), "s"); msg.IDOf(err) != "vm_not_running" {
		t.Errorf("SSH to a stopped VM: %v", err)
	}
}

// Exec quotes the folder and each argument for the guest's shell.
func TestExecAndShell(t *testing.T) {
	l, f, _ := testLima(t, "darwin")
	if err := l.Exec(t.Context(), "v", []string{"echo", "it's $HOME"}, backend.ExecOpts{Dir: "/my dir"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"limactl", "shell", "--workdir", "/", "v", "sh", "-c", `cd '/my dir' && exec 'echo' 'it'\''s $HOME'`}
	if got := f.calls[0].argv; !slices.Equal(got, want) {
		t.Errorf("Exec ran %q, want %q", got, want)
	}
	c, err := l.Shell(t.Context(), "v", "/p")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(c.Path) || !slices.Equal(c.Args[1:], []string{"shell", "--workdir", "/p", "v"}) ||
		c.Dir != l.Dirs.Data || !slices.Contains(c.Env, "LIMA_HOME="+l.Dirs.Lima) {
		t.Errorf("Shell: %+v", c)
	}
}

// Against the limactl stub: a VM Lima reports as broken (its start cut
// short before its hostagent answers) is stopped with --force, which
// limactl stop alone refuses.
func TestStubStopBroken(t *testing.T) {
	l, _, stubs := stubLima(t)
	vm := "devvm-p-0123456789abcd"
	stubs.Write(t, "instances", vm+" Broken\n")
	if err := l.Stop(t.Context(), vm); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range stubs.Calls(t) {
		if c != "limactl list --json" {
			got = append(got, c)
		}
	}
	if want := []string{"limactl stop --force " + vm}; !slices.Equal(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
}

// Against the limactl stub: VMs stop, start and go.
func TestStubLifecycle(t *testing.T) {
	l, _, stubs := stubLima(t)
	stubs.Write(t, "instances", "devvm-p-0123456789abcd Stopped\n")
	ctx := t.Context()
	vm := "devvm-p-0123456789abcd"
	if err := l.Start(ctx, vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if in, err := l.Inspect(ctx, vm); err != nil || in.Status != backend.StatusRunning {
		t.Errorf("after Start: %+v, %v", in, err)
	}
	// Started without its proxy: running, not confined.
	if ok, err := l.Confined(ctx, vm); err != nil || ok {
		t.Errorf("Confined after a direct Start: %v, %v", ok, err)
	}
	if err := l.Start(ctx, vm, backend.Confinement{}); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(ctx, vm); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(ctx, vm); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range stubs.Calls(t) {
		if c != "limactl list --json" {
			got = append(got, c)
		}
	}
	// Lima's own forwards turned off, once.
	want := []string{"limactl edit --tty=false " + vm + " --set " + noForwardsSet(), "limactl start --tty=false " + vm, "limactl stop " + vm, "limactl delete -f " + vm}
	if !slices.Equal(got, want) {
		t.Errorf("calls %q, want %q", got, want)
	}
	if _, err := os.Stat(l.startedFile(vm)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Delete left its start record: %v", err)
	}
}
