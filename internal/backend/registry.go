package backend

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Default is the backend for new VMs when neither --backend nor a file of
// defaults (config.toml, settings.json) names one.
const Default = "lima"

// Registry is the backends devvm has, by name, and the review their
// features are checked against. Host is the host they run on, whose CPUs,
// memory and free space every check of them reads.
type Registry struct {
	Review
	Host platform.Host
	// DefaultBackend returns the backend new VMs get without --backend,
	// and the file that names it, "" for Default (internal/settings);
	// nil is Default.
	DefaultBackend func() (name, file string, err error)
	byName         map[string]Backend
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
	// FlagIgnored is set when --backend was given for a VM that exists:
	// it keeps the backend it was made with.
	FlagIgnored bool
}

// For chooses the backend for vm. A VM that exists runs on the backend
// its state records. A new one gets flag (--backend), else
// DefaultBackend's.
func (r *Registry) For(d paths.Dirs, vm *state.VM, flag string) (Choice, error) {
	if vm.Exists() {
		name := vm.Get(state.KeyBackend)
		if name == "" {
			return Choice{}, &state.CorruptError{Path: d.VMState(vm.Name()), Reason: "no " + state.KeyBackend + "="}
		}
		b, ok := r.Lookup(name)
		if !ok {
			return Choice{}, msg.BackendUnknownVM(name, vm.Name())
		}
		return Choice{Backend: b, FlagIgnored: flag != "" && flag != name}, nil
	}
	if flag != "" {
		b, ok := r.Lookup(flag)
		if !ok {
			return Choice{}, msg.BackendUnknown(r.list(), flag)
		}
		return Choice{Backend: b}, nil
	}
	name, file := Default, ""
	if r.DefaultBackend != nil {
		var err error
		if name, file, err = r.DefaultBackend(); err != nil {
			return Choice{}, err
		}
	}
	b, ok := r.Lookup(name)
	switch {
	case !ok && file != "":
		return Choice{}, msg.BackendUnknownDefault(r.list(), file, name)
	case !ok:
		return Choice{}, msg.BackendUnknown(r.list(), name)
	}
	return Choice{Backend: b}, nil
}

// list is the backends' names for a message: lima, smolvm.
func (r *Registry) list() string { return strings.Join(r.Names(), ", ") }
