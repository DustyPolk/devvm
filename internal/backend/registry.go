package backend

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
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

// Register adds a backend under its name, which must be new.
func (r *Registry) Register(b Backend) error {
	name := b.Name()
	if name == "" || strings.ToLower(name) != name || strings.ContainsAny(name, " \t\n=/") {
		return fmt.Errorf("invalid backend name %q", name)
	}
	if _, dup := r.byName[name]; dup {
		return fmt.Errorf("backend %s is registered twice", name)
	}
	r.byName[name] = b
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

// Used returns the backends some VM's state names, sorted by name. A
// launch asks only these for VMs, so a backend nobody uses (not running,
// not installed) never slows one down.
func (r *Registry) Used(d paths.Dirs) ([]Backend, error) {
	used, err := state.Backends(d)
	if err != nil {
		return nil, err
	}
	var bs []Backend
	for _, b := range r.All() {
		if used[b.Name()] {
			bs = append(bs, b)
		}
	}
	return bs, nil
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
