package backend

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/state"
)

// Default is the backend new VMs get.
const Default = "lima"

// Registry is the backends devvm has, by name, and the review their
// features are checked against. Host is the host they run on, whose CPUs,
// memory and free space every check of them reads.
type Registry struct {
	Review
	Host platform.Host
	// NewVMs names the backend new VMs get; "" is Default. Tests set it
	// to a fake's name.
	NewVMs string
	byName map[string]Backend
}

// NewRegistry returns a registry holding bs.
func NewRegistry(bs ...Backend) (*Registry, error) {
	r := &Registry{byName: map[string]Backend{}}
	for _, b := range bs {
		if err := r.Register(b); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds a backend under its name, which must be new, with Caps
// that list every feature (Caps.Check). The registry hands it out behind
// its guard: CreateVM, Start, BuildImage and BuildTools refuse a spec that
// needs a feature Require would refuse, or that isn't valid. Find its
// optional roles (Forgetter) with As.
func (r *Registry) Register(b Backend) error {
	name := b.Name()
	if name == "" || strings.ToLower(name) != name || strings.ContainsAny(name, " \t\n=/") {
		return fmt.Errorf("invalid backend name %q", name)
	}
	if err := b.Caps().Check(); err != nil {
		return fmt.Errorf("backend %s: %w", name, err)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("backend %s is registered twice", name)
	}
	r.byName[name] = r.guard(b)
	return nil
}

// Lookup returns the backend by that name.
func (r *Registry) Lookup(name string) (Backend, bool) {
	b, ok := r.byName[name]
	return b, ok
}

// Names returns every backend's name, sorted.
func (r *Registry) Names() []string { return slices.Sorted(maps.Keys(r.byName)) }

// All returns every backend, sorted by name.
func (r *Registry) All() []Backend {
	var bs []Backend
	for _, n := range r.Names() {
		bs = append(bs, r.byName[n])
	}
	return bs
}

// Choice is the backend a VM runs on.
type Choice struct {
	Backend Backend
}

// For chooses the backend for vm. A VM that exists runs on the backend
// its state records, and a new one gets NewVMs, else Default.
func (r *Registry) For(vm *state.VM) (Choice, error) {
	if vm.Exists() {
		name := vm.Backend
		b, ok := r.Lookup(name)
		if !ok {
			return Choice{}, msg.BackendUnknownVM(name, vm.Name())
		}
		return Choice{Backend: b}, nil
	}
	name := cmp.Or(r.NewVMs, Default)
	b, ok := r.Lookup(name)
	if !ok {
		return Choice{}, fmt.Errorf("backend %s, which new VMs get, isn't registered", name)
	}
	return Choice{Backend: b}, nil
}
