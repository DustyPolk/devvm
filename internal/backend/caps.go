package backend

import "github.com/w3cj/devvm/internal/msg"

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
	FeaturePorts            Feature = "ports"             // forwarding guest ports to host loopback
	FeatureAgentForwarding  Feature = "agent-forwarding"  // ssh-agent forwarding
	FeatureCPUs             Feature = "cpus"              // setting a VM's CPUs
	FeatureMemory           Feature = "memory"            // setting a VM's memory
	FeatureDisk             Feature = "disk"              // setting a VM's disk size
)

// Features is every feature, in the backends page's order.
var Features = []Feature{
	FeatureBuildImage, FeatureBuildConfinement, FeatureFrom, FeatureClone,
	FeatureProjectMount, FeatureAgentFolders, FeatureToolsShare,
	FeatureEgressProxy, FeatureSSH, FeaturePorts, FeatureAgentForwarding,
	FeatureCPUs, FeatureMemory, FeatureDisk,
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

// Caps is how a backend has each feature. A feature it doesn't list is None.
type Caps map[Feature]Support

// Require returns nil when b has every feature usable, else the refusal
// for the first one it lacks or only emulates unreviewed, naming vm.
func Require(b Backend, vm string, features ...Feature) error {
	caps := b.Caps()
	for _, f := range features {
		switch s := caps[f]; {
		case s.Usable():
		case s.IsEmulated():
			return msg.BackendUnreviewed(b.Name(), string(f), vm)
		default:
			return msg.BackendLacks(b.Name(), string(f), vm)
		}
	}
	return nil
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
	return append(fs, s.Net.Features()...)
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
	if p.All || len(p.Ranges) > 0 {
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
