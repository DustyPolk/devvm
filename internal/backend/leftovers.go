package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

// Locator is a backend whose VMs live somewhere that can change between
// runs of devvm: Lima's home (DEVVM_LIMA_HOME), smolvm's user on macOS
// (HOME), Docker's engine (DOCKER_HOST, DOCKER_CONTEXT). A VM records
// where it was made (LocationOf), so a run that looks elsewhere never
// takes it for one its backend lost. A backend whose VMs can't move
// doesn't implement it.
type Locator interface {
	// Location says where this run's VMs live, and whether that's the
	// backend's default.
	Location() (where string, isDefault bool)
}

// LocationOf is where b's VMs live in this run, as a VM's state records
// it (a hash, so any path fits), and whether that's b's default; "" and
// true for a backend that isn't a Locator.
func LocationOf(b Backend) (string, bool) {
	l, ok := b.(Locator)
	if !ok {
		return "", true
	}
	where, isDefault := l.Location()
	sum := sha256.Sum256([]byte(where))
	return hex.EncodeToString(sum[:8]), isDefault
}

// Forgetter is a backend that keeps files or resources of its own for a
// VM besides the VM (its port records, its ssh key, Docker's volumes and
// networks), which devvm gc --orphans deletes once the backend no longer
// has the VM, as Delete does after deleting it.
type Forgetter interface {
	// Forget deletes what the backend keeps for vm, which it must not
	// have: a VM it has is refused, and nothing is deleted.
	Forget(ctx context.Context, vm string) error
}

// Stray is a VM with devvm's label that this devvm has no state for: one
// made by a devvm with another data folder (a sandbox's, a test's), or
// one whose state is gone.
type Stray struct {
	Name string
	Data string // the data folder its label names
}

// StrayFinder is a backend that lists its strays (devvm space), which
// devvm never deletes: they may be another devvm's.
type StrayFinder interface {
	Strays(ctx context.Context) ([]Stray, error)
}
