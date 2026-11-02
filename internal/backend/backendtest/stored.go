package backendtest

import "github.com/w3cj/devvm/internal/backend"

// Stored is a fake whose disks are on the host, in Dirs
// (backend.Storage), with Floor its floors: DefaultFloors when it's
// zero. Its Fake may be nil when nothing else of it is called.
type Stored struct {
	*Fake
	Dirs  []string
	Floor backend.Floors
}

var _ backend.Storage = Stored{}

// DefaultFloors returns a Stored's floors unless it sets its own: 12 GiB
// to build base, 8 for another image, 4 for a VM, 3 for shared tools and
// 1 more for each.
func DefaultFloors() backend.Floors {
	return backend.Floors{Base: 12 << 30, Image: 8 << 30, VM: 4 << 30, Tools: 3 << 30, PerTool: 1 << 30}
}

// DiskDirs returns Dirs.
func (s Stored) DiskDirs() []string { return s.Dirs }

// Floors returns Floor, or DefaultFloors when it's zero.
func (s Stored) Floors() backend.Floors {
	if s.Floor == (backend.Floors{}) {
		return DefaultFloors()
	}
	return s.Floor
}
