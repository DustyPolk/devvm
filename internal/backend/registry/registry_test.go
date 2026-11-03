package registry

import (
	"context"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/backend/vmssh"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/settings"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/ubuntu"
)

// quietUI prints nothing, runs commands as they are, counts no steps and
// keeps no cleanups.
type quietUI struct{}

func (quietUI) LoggedCmd(_ context.Context, _ msg.M, cmd *exec.Cmd) error { return cmd.Run() }
func (quietUI) Info(msg.M)                                                {}
func (quietUI) Note(msg.M)                                                {}
func (quietUI) Warn(msg.M)                                                {}
func (quietUI) LogNote()                                                  {}
func (quietUI) WithSteps(ctx context.Context, _ int) context.Context      { return ctx }
func (quietUI) AddCleanup(func(context.Context)) func()                   { return func() {} }
func (quietUI) Interrupted() bool                                         { return false }
func (quietUI) StdinIsTerminal() bool                                     { return false }

// A test build's overrides reach the registry and every backend; without
// them, the backends keep their own, whatever the environment says.
func TestOverrides(t *testing.T) {
	for k, v := range map[string]string{
		"DEVVM_KVM": "/x/kvm", "DEVVM_UBUNTU_URL": "file:///x/ubuntu", "DEVVM_UNREVIEWED": "smolvm:egress-proxy",
		platform.TestCPUs: "1234",
	} {
		t.Setenv(k, v)
	}
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	o := Overrides{
		Host:       platform.Overrides{CPUs: "1234", MemoryGiB: "4321", FreeGiB: "0"},
		Unreviewed: "smolvm:egress-proxy",
		KVM:        "/x/kvm", UbuntuURL: "file:///x/ubuntu/",
	}
	for _, c := range []struct {
		o    Overrides
		want bool
	}{{Overrides{}, false}, {o, true}} {
		r, err := New(d, assets.FS, quietUI{}, c.o)
		if err != nil {
			t.Fatal(err)
		}
		lb, _ := r.Lookup("lima")
		sb, _ := r.Lookup("smolvm")
		l, s := lb.(*lima.Lima), sb.(*smolvm.Smolvm)
		mem, _ := r.Host.Memory()
		free, _ := r.Host.FreeSpace(t.TempDir())
		for what, got := range map[string]bool{
			"let through":           r.Unreviewed["smolvm:egress-proxy"],
			"lima's KVM":            l.KVM == "/x/kvm",
			"smolvm's KVM":          s.KVM == "/x/kvm",
			"lima's Ubuntu":         l.UbuntuURL == "file:///x/ubuntu",
			"the registry's CPUs":   r.Host.CPUs() == 1234,
			"the registry's memory": mem == 4321<<30,
			"the registry's free":   free == 0,
			"lima's CPUs":           l.CPUs() == 1234,
			"smolvm's CPUs":         s.CPUs() == 1234,
		} {
			if got != c.want {
				t.Errorf("overrides %v: %s overridden = %v", c.want, what, got)
			}
		}
		if !c.want && (l.KVM != "/dev/kvm" || s.KVM != "/dev/kvm" || l.UbuntuURL != ubuntu.DefaultURL) {
			t.Errorf("none: KVM %q %q, Ubuntu %q", l.KVM, s.KVM, l.UbuntuURL)
		}
	}
}

// lima is registered, and it's what a new VM gets by default.
func TestLimaIsTheDefault(t *testing.T) {
	t.Parallel()
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	r, err := New(d, assets.FS, quietUI{}, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("lima"); !ok {
		t.Fatalf("no lima in %q", r.Names())
	}
	// Names, which --backend is checked against before anything runs,
	// are the registry's own.
	if got, want := strings.Join(Names(), " "), strings.Join(r.Names(), " "); got != want {
		t.Errorf("Names() = %q, the registry has %q", got, want)
	}
	vm, err := state.ReadVM(d, "devvm-p-0123456789abcd")
	if err != nil {
		t.Fatal(err)
	}
	c, err := r.For(d, vm, "")
	if err != nil || c.Backend.Name() != backend.Default || backend.Default != "lima" {
		t.Errorf("For a new VM: %+v, %v", c, err)
	}
}

// smolvm is experimental, and Lima and a name that isn't a backend aren't.
func TestExperimental(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{"smolvm": true, "lima": false, "nope": false, "": false} {
		if got := Experimental(name); got != want {
			t.Errorf("Experimental(%q) = %v, want %v", name, got, want)
		}
	}
}

// A new VM's backend is settings.json's, then config.toml's over it, and
// one that isn't registered names the file that set it.
func TestDefaultBackendFiles(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	r, err := New(d, assets.FS, quietUI{}, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	vm, err := state.ReadVM(d, "devvm-p-0123456789abcd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Save(d, settings.Values{Backend: settings.Of("smolvm")}); err != nil {
		t.Fatal(err)
	}
	if c, err := r.For(d, vm, ""); err != nil || c.Backend.Name() != "smolvm" {
		t.Errorf("settings.json's: %+v, %v", c, err)
	}
	for _, tc := range []struct{ file, want string }{
		{"backend = \"lima\"\n", "lima"},
		{"backend = \"qemu\"\n", "backend_unknown_default backends=lima, smolvm file=" + d.ConfigFile() + " name=qemu"},
	} {
		if err := os.WriteFile(d.ConfigFile(), []byte(tc.file), 0o600); err != nil {
			t.Fatal(err)
		}
		c, err := r.For(d, vm, "")
		got := msg.NamesOf(err)
		if err == nil {
			got = c.Backend.Name()
		}
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.file, got, tc.want)
		}
	}
}

// homeMax is the longest home folder whose data folder (the default,
// ~/.local/share/devvm) keeps every socket devvm makes within the limit:
// a VM's folder holds none, and those in the run folder are named by
// hashes, so no VM's name lengthens them.
const homeMax = 35

// Every socket devvm makes for a VM, the longest VM name's included, fits
// a socket's path under the longest home: devvm's agent, the port
// forwarders' control sockets (ssh adds 17 to those), and a link session's
// sockets and control sockets.
func TestSocketsFit(t *testing.T) {
	home := "/" + strings.Repeat("h", homeMax-1)
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("DEVVM_LIMA_HOME", "")
	d, err := paths.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(d, assets.FS, quietUI{}, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	vm := "devvm-" + strings.Repeat("a", 20) + "-0123456789abcd" // the longest folder part
	target := "devvm-" + strings.Repeat("b", 20) + "-0123456789abce"
	lb, _ := r.Lookup("lima")
	sb, _ := r.Lookup("smolvm")
	l, s := lb.(*lima.Lima), sb.(*smolvm.Smolvm)
	links := vmssh.Links{VM: vm, Dir: d.Run()}
	ctls := []string{
		l.Forwards.Control(vm), s.Forwards.Control(vm),
		links.Control("", ""), links.Control(target, "12345 2026-11-03T00:00:00.000000000Z"),
	}
	for _, p := range ctls {
		if len(p) > vmssh.ControlMax {
			t.Errorf("%s: %d characters, over %d", p, len(p), vmssh.ControlMax)
		}
	}
	socks := []string{d.AgentSock(), links.Socket(vmssh.Link{Addr: netip.MustParseAddr("127.77.255.255"), Port: 65535})}
	for _, p := range socks {
		if len(p) >= paths.SocketMax {
			t.Errorf("%s: %d characters, %d at most", p, len(p), paths.SocketMax-1)
		}
	}
	// None of them is in the VM's folder, and none's length is the VM
	// name's: the same for the shortest name.
	for _, p := range slices.Concat(ctls, socks) {
		if strings.HasPrefix(p, d.VMs()) || strings.Contains(p, vm) {
			t.Errorf("%s: in a VM's folder, or named by the VM", p)
		}
	}
	short := vmssh.Links{VM: "devvm-a-0123456789abcd", Dir: d.Run()}
	if len(short.Control("", "")) != len(links.Control("", "")) || len(l.Forwards.Control("devvm-a-0123456789abcd")) != len(l.Forwards.Control(vm)) {
		t.Error("a control socket's length depends on the VM's name")
	}
	// One more character of home and a control socket is too long.
	if over := len(l.Forwards.Control(vm)) + 1; over <= vmssh.ControlMax && len(links.Control(target, "x"))+1 <= vmssh.ControlMax &&
		len(s.Forwards.Control(vm))+1 <= vmssh.ControlMax {
		t.Errorf("homeMax %d isn't the longest: raise it", homeMax)
	}
}
