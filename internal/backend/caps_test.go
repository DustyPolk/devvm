package backend_test

import (
	"slices"
	"testing"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/backendtest"
	"github.com/w3cj/devvm/internal/msg"
)

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
	t.Parallel()
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
		if got := msg.NamesOf(backend.Review{}.Require(b, "devvm-x", tc.features...)); got != tc.want {
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
	if len(seen) != 19 {
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
		{
			"the nix layer", backend.ImageSpec{Image: "node+nix", Net: backend.NetInternet}.Features(),
			f(backend.FeatureBuildImage, backend.FeatureNixLayer, backend.FeatureBuildConfinement),
		},
		{"a VM of docker and nix", backend.ImageFeatures("docker+nix+foo"), f(backend.FeatureDockerLayer, backend.FeatureNixLayer)},
		{"a VM of neither", backend.ImageFeatures("node+nixos"), nil},
		{"a --clone VM forwarding nothing", backend.VMSpec{Name: "v"}.Features(), f(backend.FeatureClone)},
		{"a VM with everything", backend.VMSpec{
			Name: "v", Project: "/p", AgentDirs: []string{"/h/.claude"}, Tools: "/t", ToolsAt: "/opt/devvm",
			AgentForwarding: true, Size: backend.Size{CPUs: 2, MemoryGiB: 4, DiskGiB: 60},
		}.Features(), f(backend.FeatureClone, backend.FeatureProjectMount, backend.FeatureAgentFolders, backend.FeatureToolsShare,
			backend.FeatureAgentForwarding, backend.FeatureCPUs, backend.FeatureMemory, backend.FeatureDisk)},
		{"every port", backend.Ports{All: true}.Features(), f(backend.FeaturePorts, backend.FeaturePortsAll)},
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
	t.Parallel()
	b := backendtest.New("fake", backend.Caps{backend.FeatureEgressProxy: backend.Emulated(false)})
	c := backend.Confinement{Net: backend.NetProxy}
	want := "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-x"
	if got := msg.NamesOf(backend.Review{}.Require(b, "devvm-x", c.Features()...)); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if err := (backend.Review{}).Require(b, "devvm-x", backend.Confinement{}.Features()...); err != nil {
		t.Errorf("direct: %v", err)
	}
}

// DEVVM_UNREVIEWED lets through only the unreviewed emulations it names,
// on their own backend, warning each time; never a feature a backend lacks.
func TestUnreviewedLetThrough(t *testing.T) {
	t.Parallel()
	b := backendtest.New("fake", backend.Caps{backend.FeatureEgressProxy: backend.Emulated(false)})
	var warned msg.Recorder
	r := backend.Review{
		Unreviewed: backend.ParseUnreviewed("lima:egress-proxy, fake:ssh"),
		Warn:       warned.Add,
	}
	if got := msg.NamesOf(r.Require(b, "devvm-x", backend.FeatureEgressProxy)); got != "backend_unreviewed backend=fake feature=egress-proxy vm=devvm-x" {
		t.Errorf("another backend's: %s", got)
	}
	if got := msg.NamesOf(r.Require(b, "devvm-x", backend.FeatureSSH)); got != "backend_lacks backend=fake feature=ssh vm=devvm-x" {
		t.Errorf("a feature it lacks: %s", got)
	}
	if r.Allowed(b, backend.FeatureEgressProxy) || r.Allowed(b, backend.FeatureSSH) {
		t.Error("Allowed: not named, or lacking")
	}
	r.Unreviewed = backend.ParseUnreviewed("fake:egress-proxy")
	if err := r.Require(b, "devvm-x", backend.FeatureEgressProxy); err != nil ||
		!slices.Equal(warned.Names(), []string{"backend_unreviewed_used backend=fake feature=egress-proxy vm=devvm-x"}) {
		t.Errorf("named: %v, warned %q", err, warned.Names())
	}
	// Allowed says the same, without a warning.
	if !r.Allowed(b, backend.FeatureEgressProxy) || len(warned.Names()) != 1 {
		t.Errorf("Allowed: warned %q", warned.Names())
	}
}
