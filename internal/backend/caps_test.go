package backend_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
)

// names returns err's message names (backend_lacks backend=x …), or ""
// for nil. It fails the test for an error that isn't a message.
func names(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	var m msg.M
	if !errors.As(err, &m) {
		t.Fatalf("not a message: %v", err)
	}
	return m.Names()
}

func TestSupport(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		s                            backend.Support
		native, emulated, ok, review bool
		shown                        string
	}{
		{"native", backend.Native, true, false, true, false, "native"},
		{"none", backend.None, false, false, false, false, "none"},
		{"emulated, reviewed", backend.Emulated(true), false, true, true, true, "emulated"},
		{"emulated, unreviewed", backend.Emulated(false), false, true, false, false, "unreviewed"},
	} {
		s := tc.s
		if s.IsNative() != tc.native || s.IsEmulated() != tc.emulated || s.Usable() != tc.ok || s.Reviewed() != tc.review {
			t.Errorf("%s: native %v, emulated %v, usable %v, reviewed %v", tc.name, s.IsNative(), s.IsEmulated(), s.Usable(), s.Reviewed())
		}
		if s.String() != tc.shown {
			t.Errorf("%s: shown as %q, want %q", tc.name, s.String(), tc.shown)
		}
	}
}

func TestRequire(t *testing.T) {
	b := backendtest.New("fake", backend.Caps{
		backend.FeatureClone:           backend.Native,
		backend.FeatureSSH:             backend.Emulated(true),
		backend.FeatureEgressProxy:     backend.Emulated(false),
		backend.FeatureDisk:            backend.None,
		backend.FeatureAgentForwarding: backend.Native,
	})
	for _, tc := range []struct {
		name     string
		features []backend.Feature
		want     string
	}{
		{"nothing", nil, ""},
		{"native", []backend.Feature{backend.FeatureClone}, ""},
		{"reviewed emulation", []backend.Feature{backend.FeatureSSH}, ""},
		{"unreviewed emulation", []backend.Feature{backend.FeatureEgressProxy}, "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-x"},
		{"none", []backend.Feature{backend.FeatureDisk}, "backend_lacks backend=fake feature=disk vm=devvm-x"},
		{"not listed", []backend.Feature{backend.FeaturePorts}, "backend_lacks backend=fake feature=ports vm=devvm-x"},
		{"the first refused", []backend.Feature{backend.FeatureClone, backend.FeaturePorts, backend.FeatureEgressProxy}, "backend_lacks backend=fake feature=ports vm=devvm-x"},
	} {
		if got := names(t, backend.Require(b, "devvm-x", tc.features...)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEveryFeatureIsListed(t *testing.T) {
	seen := map[backend.Feature]bool{}
	for _, f := range backend.Features {
		if seen[f] {
			t.Errorf("%s is listed twice", f)
		}
		seen[f] = true
	}
	if len(seen) != 16 {
		t.Errorf("%d features; a new one goes in Features and on the backends page", len(seen))
	}
}

func TestFeaturesNeeded(t *testing.T) {
	f := func(fs ...backend.Feature) []backend.Feature { return fs }
	for _, tc := range []struct {
		name string
		got  []backend.Feature
		want []backend.Feature
	}{
		{"a layer", backend.ImageSpec{Net: backend.NetInternet}.Features(), f(backend.FeatureBuildImage, backend.FeatureBuildConfinement)},
		{
			"base --from, offline, sized", backend.ImageSpec{From: &backend.From{Ref: "x"}, MemoryGiB: 4, Net: backend.NetNone}.Features(),
			f(backend.FeatureBuildImage, backend.FeatureFrom, backend.FeatureMemory, backend.FeatureBuildConfinement),
		},
		{"a --clone VM forwarding nothing", backend.VMSpec{Name: "v"}.Features(), f(backend.FeatureClone)},
		{"a VM with everything", backend.VMSpec{
			Name: "v", Project: "/p", AgentDirs: []string{"/h/.claude"}, Tools: "/t", ToolsAt: "/opt/devvm",
			AgentForwarding: true, Size: backend.Size{CPUs: 2, MemoryGiB: 4, DiskGiB: 60}, Ports: backend.Ports{All: true},
		}.Features(), f(backend.FeatureClone, backend.FeatureProjectMount, backend.FeatureAgentFolders, backend.FeatureToolsShare,
			backend.FeatureAgentForwarding, backend.FeatureCPUs, backend.FeatureMemory, backend.FeatureDisk, backend.FeaturePorts,
			backend.FeaturePortsAll)},
		{"some ports", backend.Ports{Ranges: []backend.PortRange{{5173, 5173}}}.Features(), f(backend.FeaturePorts)},
		{"no ports", backend.Ports{}.Features(), nil},
		{"direct", backend.Confinement{Net: backend.NetDirect}.Features(), nil},
		{"through the proxy", backend.Confinement{Net: backend.NetProxy, ProxyPort: 3128}.Features(), f(backend.FeatureEgressProxy)},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

// A backend that only emulates network policy, unreviewed, can't start a
// VM with it, but can start one without.
func TestUnreviewedEgressRefused(t *testing.T) {
	b := backendtest.New("fake", backend.Caps{backend.FeatureEgressProxy: backend.Emulated(false)})
	c := backend.Confinement{Net: backend.NetProxy}
	want := "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-x"
	if got := names(t, backend.Require(b, "devvm-x", c.Features()...)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if err := backend.Require(b, "devvm-x", backend.Confinement{}.Features()...); err != nil {
		t.Errorf("direct: %v", err)
	}
}
