package cli

import (
	"testing"

	"github.com/w3cj/devvm/assets"
	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/testbuild"
)

// A release, as this test binary is (built without the test build's
// flag), reads none of the test overrides, whatever its environment
// says: its mise is the pinned one, its backends check the real
// /dev/kvm and its host is this machine. The CLI tests' test build reads
// them all (the scripts).
func TestReleaseReadsNoOverrides(t *testing.T) {
	if testbuild.On() {
		t.Skip("a test build")
	}
	if o := readOverrides(func(string) string { return "/x" }); o != (Overrides{}) {
		t.Errorf("readOverrides = %+v", o)
	}
	for _, k := range []string{
		"DEVVM_HOST_MISE", "DEVVM_KVM", "DEVVM_UBUNTU_URL", "DEVVM_UNREVIEWED",
		platform.TestCPUs, platform.TestMemoryGiB, platform.TestFreeGiB, platform.TestListWait,
	} {
		t.Setenv(k, "1")
	}
	d := paths.Dirs{Data: t.TempDir(), Config: t.TempDir(), Lima: t.TempDir()}
	u, _, _ := testUI()
	if tl := newTools(d, assets.FS, u, thisHost()); tl.Mise != "" {
		t.Errorf("mise = %q", tl.Mise)
	}
	r, err := newRegistry(d, assets.FS, u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r.Lookup("lima")
	if l, _ := backend.As[*lima.Lima](b); l.KVM != "/dev/kvm" || len(r.Unreviewed) != 0 {
		t.Errorf("KVM %q, let through %v", l.KVM, r.Unreviewed)
	}
	if got, want := thisHost().CPUs(), (platform.Host{}).CPUs(); got != want {
		t.Errorf("the host's CPUs = %d, want %d", got, want)
	}
}
