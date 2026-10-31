package backend

import (
	"context"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
)

// Feature is something a backend may have. Its name is the one the backends
// page lists.
type Feature string

// Features, in the backends page's order.
const (
	FeatureBuildImage       Feature = "build-image"       // build images from the layer and feature scripts
	FeatureBuildConfinement Feature = "build-confinement" // builds reach the internet but not host loopback (NetInternet), or nothing (NetNone)
	FeatureFrom             Feature = "from"              // base from a container image (--from)
	FeatureClone            Feature = "clone"             // a VM from an image
	FeatureProjectMount     Feature = "project-mount"     // the project folder, read-write
	FeatureAgentFolders     Feature = "agent-folders"     // agents' host folders, read-only
	FeatureToolsShare       Feature = "tools-share"       // the shared tools folder, read-only
	FeatureEgressProxy      Feature = "egress-proxy"      // egress only through devvm's proxy (NetProxy)
	FeatureSSH              Feature = "ssh"               // ssh for editors
	FeaturePorts            Feature = "ports"             // forwarding listed guest ports to host loopback
	FeaturePortsAll         Feature = "ports-all"         // forwarding every guest port, whatever address it listens on
	FeatureAgentForwarding  Feature = "agent-forwarding"  // ssh-agent forwarding, chosen when a VM is made
	FeatureAgentToggle      Feature = "agent-toggle"      // turning a VM's ssh-agent forwarding on or off later
	FeatureCPUs             Feature = "cpus"              // setting a VM's CPUs
	FeatureMemory           Feature = "memory"            // setting a VM's memory
	FeatureDisk             Feature = "disk"              // setting a VM's disk size
	FeatureDockerLayer      Feature = "docker-layer"      // the docker layer: Docker's engine running in the VM
	FeatureNixLayer         Feature = "nix-layer"         // the nix layer: Nix's daemon and store in the VM
)

// Features is every feature, in the backends page's order.
var Features = []Feature{
	FeatureBuildImage, FeatureBuildConfinement, FeatureFrom, FeatureClone,
	FeatureProjectMount, FeatureAgentFolders, FeatureToolsShare,
	FeatureEgressProxy, FeatureSSH, FeaturePorts, FeaturePortsAll,
	FeatureAgentForwarding, FeatureAgentToggle,
	FeatureCPUs, FeatureMemory, FeatureDisk, FeatureDockerLayer, FeatureNixLayer,
}

// Support is how a backend has a feature: natively, emulated (usable only
// once the emulation's review has passed), or not at all.
type Support struct {
	kind     kind
	reviewed bool
}

type kind int

const (
	none kind = iota
	native
	emulated
)

// Native is a feature the backend has itself.
var Native = Support{kind: native}

// None is a feature the backend lacks.
var None = Support{}

// Emulated is a feature devvm builds on the backend from other parts.
// reviewed is whether its review passed: a design note, a threat check and
// e2e checks proving the guarantee. Until then it's refused.
func Emulated(reviewed bool) Support { return Support{kind: emulated, reviewed: reviewed} }

// IsNative reports whether the feature is the backend's own.
func (s Support) IsNative() bool { return s.kind == native }

// IsEmulated reports whether the feature is emulated, reviewed or not.
func (s Support) IsEmulated() bool { return s.kind == emulated }

// Reviewed reports whether an emulated feature's review passed.
func (s Support) Reviewed() bool { return s.reviewed }

// Usable reports whether devvm uses the feature: native, or emulated and
// reviewed.
func (s Support) Usable() bool { return s.kind == native || s.kind == emulated && s.reviewed }

// String is the support as devvm doctor lists it: native, emulated,
// unreviewed (emulated, refused until its review passes) or none.
func (s Support) String() string {
	switch {
	case s.IsNative():
		return "native"
	case s.Usable():
		return "emulated"
	case s.IsEmulated():
		return "unreviewed"
	}
	return "none"
}

// Caps is how a backend has each feature. A feature it doesn't list is None.
type Caps map[Feature]Support

// Review checks backends' features against their review. Its zero value
// refuses every unreviewed emulation.
type Review struct {
	// Unreviewed is the unreviewed emulations Require lets through anyway,
	// as backend:feature (smolvm:egress-proxy): for the end-to-end checks
	// that an emulation's review needs, which set DEVVM_UNREVIEWED.
	Unreviewed map[string]bool
	// Warn is told each time one is let through.
	Warn func(msg.M)
}

// ParseUnreviewed reads DEVVM_UNREVIEWED's value: backend:feature pairs,
// comma-separated.
func ParseUnreviewed(v string) map[string]bool {
	out := map[string]bool{}
	for p := range strings.SplitSeq(v, ",") {
		if p = strings.TrimSpace(p); strings.Contains(p, ":") {
			out[p] = true
		}
	}
	return out
}

// Require returns nil when b has every feature usable or let through, else
// the refusal for the first one it lacks or only emulates unreviewed,
// naming vm.
func (r Review) Require(b Backend, vm string, features ...Feature) error {
	caps := b.Caps()
	for _, f := range features {
		switch s := caps[f]; {
		case s.Usable():
		case s.IsEmulated() && r.Unreviewed[b.Name()+":"+string(f)]:
			r.Warn(msg.BackendUnreviewedUsed(b.Name(), string(f), vm))
		case s.IsEmulated():
			return msg.BackendUnreviewed(b.Name(), string(f), vm)
		default:
			return msg.BackendLacks(b.Name(), string(f), vm)
		}
	}
	return nil
}

// Allowed reports whether Require lets f through on b, without a warning:
// for what uses a feature a VM already got (Require told then).
func (r Review) Allowed(b Backend, f Feature) bool {
	s := b.Caps()[f]
	return s.Usable() || s.IsEmulated() && r.Unreviewed[b.Name()+":"+string(f)]
}

// Features returns what building the image needs.
func (s ImageSpec) Features() []Feature {
	fs := []Feature{FeatureBuildImage}
	if s.From != nil {
		fs = append(fs, FeatureFrom)
	}
	if s.MemoryGiB > 0 {
		fs = append(fs, FeatureMemory)
	}
	// A backend without nix-layer can't run the layer's script either: its
	// installer needs the daemon's init system.
	if image.HasLayer(s.Image, image.NixLayer) {
		fs = append(fs, FeatureNixLayer)
	}
	return append(fs, s.Net.Features()...)
}

// ImageFeatures returns what a VM of the image (layers joined with +)
// needs for its layers: docker-layer when it has the docker layer,
// nix-layer when it has the nix layer.
func ImageFeatures(img string) []Feature {
	var fs []Feature
	if image.HasLayer(img, image.DockerLayer) {
		fs = append(fs, FeatureDockerLayer)
	}
	if image.HasLayer(img, image.NixLayer) {
		fs = append(fs, FeatureNixLayer)
	}
	return fs
}

// DockerLayerVM is a backend whose VMs run the docker layer only when made
// for it, under a runtime that gives root in them more than its other VMs
// get (Docker's: Sysbox, FeatureDockerLayer). Root in one can change its
// own network, so it gets no network policy.
type DockerLayerVM interface {
	// DockerLayer names the runtime a VM (vm) with the docker layer runs
	// under (sysbox), or says why the backend can't run it here, in a
	// message naming vm.
	DockerLayer(vm string) (string, error)
	// SetDockerLayer makes a stopped VM one for the docker layer, added to
	// it in place, from its next start (on); or, the layer's script having
	// failed, one as it was before (off).
	SetDockerLayer(ctx context.Context, vm string, on bool) error
}

// Features returns what the tools build needs.
func (s ToolsSpec) Features() []Feature {
	return append([]Feature{FeatureBuildImage}, s.Net.Features()...)
}

// Features returns what creating the VM needs.
func (s VMSpec) Features() []Feature {
	fs := []Feature{FeatureClone}
	if s.Project != "" {
		fs = append(fs, FeatureProjectMount)
	}
	if len(s.AgentDirs) > 0 {
		fs = append(fs, FeatureAgentFolders)
	}
	if s.Tools != "" {
		fs = append(fs, FeatureToolsShare)
	}
	if s.AgentForwarding {
		fs = append(fs, FeatureAgentForwarding)
	}
	fs = append(fs, s.Size.Features()...)
	return append(fs, s.Ports.Features()...)
}

// Features returns what setting the size needs.
func (s Size) Features() []Feature {
	var fs []Feature
	if s.CPUs > 0 {
		fs = append(fs, FeatureCPUs)
	}
	if s.MemoryGiB > 0 {
		fs = append(fs, FeatureMemory)
	}
	if s.DiskGiB > 0 {
		fs = append(fs, FeatureDisk)
	}
	return fs
}

// Features returns what forwarding the ports needs: nothing for none.
func (p Ports) Features() []Feature {
	switch {
	case p.All:
		return []Feature{FeaturePorts, FeaturePortsAll}
	case len(p.Ranges) > 0:
		return []Feature{FeaturePorts}
	}
	return nil
}

// Features returns what starting with the network needs.
func (n Net) Features() []Feature {
	switch n {
	case NetProxy:
		return []Feature{FeatureEgressProxy}
	case NetInternet, NetNone:
		return []Feature{FeatureBuildConfinement}
	case NetDirect:
	}
	return nil
}

// Features returns what starting with the confinement needs.
func (c Confinement) Features() []Feature { return c.Net.Features() }
