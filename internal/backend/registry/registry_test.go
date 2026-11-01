package registry

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/docker"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
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

// DEVVM_UNREVIEWED, DEVVM_DOCKER_KIND and the host's overrides count only
// in a test build: a release ignores them.
func TestUnreviewedOnlyInTestBuilds(t *testing.T) {
	t.Parallel()
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	env := map[string]string{
		"DEVVM_UNREVIEWED": "smolvm:egress-proxy", "DEVVM_DOCKER_KIND": "linux",
		platform.TestCPUs: "1234", platform.TestMemoryGiB: "4321", platform.TestFreeGiB: "0",
	}
	for _, c := range []struct {
		build string
		want  bool
	}{{"", false}, {"yes", false}, {"allowed", true}} {
		r, err := newFor(d, assets.FS, quietUI{}, c.build, func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Unreviewed["smolvm:egress-proxy"]; got != c.want {
			t.Errorf("build %q: let through = %v", c.build, got)
		}
		b, _ := r.Lookup("docker")
		if got := b.(*docker.Docker).Kind == "linux"; got != c.want {
			t.Errorf("build %q: docker's kind set = %v", c.build, got)
		}
		checkHost(t, c.build, r, c.want)
	}
}

// checkHost checks that the host's overrides reach the registry's Host,
// Host's, and Lima's and smolvm's sizing only when want.
func checkHost(t *testing.T, build string, r *backend.Registry, want bool) {
	t.Helper()
	l, _ := r.Lookup("lima")
	s, _ := r.Lookup("smolvm")
	mem, _ := r.Host.Memory()
	free, _ := r.Host.FreeSpace(t.TempDir())
	for what, got := range map[string]bool{
		"the registry's CPUs":   r.Host.CPUs() == 1234,
		"the registry's memory": mem == 4321<<30,
		"the registry's free":   free == 0,
		"Host's CPUs":           hostFor(build, func(k string) string { return map[string]string{platform.TestCPUs: "1234"}[k] }).CPUs() == 1234,
		"lima's CPUs":           l.(*lima.Lima).CPUs() == 1234,
		"smolvm's CPUs":         s.(*smolvm.Smolvm).CPUs() == 1234,
	} {
		if got != want {
			t.Errorf("build %q: %s overridden = %v", build, what, got)
		}
	}
}

// A release's Host, this test binary's (built without the -X flag),
// ignores the overrides.
func TestReleaseHostIgnoresOverrides(t *testing.T) {
	t.Setenv(platform.TestCPUs, "1234")
	if Host().CPUs() == 1234 {
		t.Error("the release build read DEVVM_TEST_CPUS")
	}
}

// lima is registered, and it's what a new VM gets by default.
func TestLimaIsTheDefault(t *testing.T) {
	t.Parallel()
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	r, err := New(d, assets.FS, quietUI{})
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
