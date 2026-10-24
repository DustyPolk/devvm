// Package backendtest has a fake backend for tests: it records every call
// and answers from what the test preset.
package backendtest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/w3cj/devvm/internal/backend"
)

// Call is one call to the fake: the method's name and its arguments after
// the context.
type Call struct {
	Method string
	Args   []any
}

// Fake is a backend that runs nothing. Set its fields before use: they're
// what it answers, and it keeps Instances and ImageKeys current as VMs and
// images are made, changed and deleted. Calls returns every call, in order.
type Fake struct {
	BackendName  string
	Capabilities backend.Caps
	NameLimit    int                         // NameMax's answer
	Instances    map[string]backend.Instance // by name
	ImageKeys    []string
	Built        backend.Built     // BuildImage's answer
	Target       backend.SSHTarget // SSH's answer
	ShellCmd     backend.Command   // Shell's answer
	Unconfined   bool              // Confined answers false
	Errs         map[string]error  // by method name: returned in place of its answer, changing nothing

	mu    sync.Mutex
	calls []Call
}

var _ backend.Backend = (*Fake)(nil)

// New returns a fake named name with these capabilities.
func New(name string, caps backend.Caps) *Fake {
	return &Fake{BackendName: name, Capabilities: caps, Instances: map[string]backend.Instance{}}
}

// Calls returns the calls so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Methods returns the names of the methods called so far, in order.
func (f *Fake) Methods() []string {
	var ms []string
	for _, c := range f.Calls() {
		ms = append(ms, c.Method)
	}
	return ms
}

// record notes a call and returns the error preset for its method. The
// caller holds the lock.
func (f *Fake) record(method string, args ...any) error {
	f.calls = append(f.calls, Call{method, args})
	return f.Errs[method]
}

// change applies fn to an instance. The caller holds the lock.
func (f *Fake) change(vm string, fn func(*backend.Instance)) error {
	in, ok := f.Instances[vm]
	if !ok {
		return fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	}
	fn(&in)
	f.Instances[vm] = in
	return nil
}

// Name returns BackendName.
func (f *Fake) Name() string { return f.BackendName }

// Caps returns Capabilities.
func (f *Fake) Caps() backend.Caps { return f.Capabilities }

// NameMax returns NameLimit.
func (f *Fake) NameMax() int { return f.NameLimit }

// Check records the call.
func (f *Fake) Check(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.record("Check")
}

// BuildImage records the call, adds the key to ImageKeys and returns Built.
func (f *Fake) BuildImage(_ context.Context, spec backend.ImageSpec) (backend.Built, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("BuildImage", spec); err != nil {
		return backend.Built{}, err
	}
	if !slices.Contains(f.ImageKeys, spec.Key) {
		f.ImageKeys = append(f.ImageKeys, spec.Key)
	}
	return f.Built, nil
}

// Images records the call and returns ImageKeys.
func (f *Fake) Images(context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Images"); err != nil {
		return nil, err
	}
	return slices.Clone(f.ImageKeys), nil
}

// DeleteImage records the call and drops the key from ImageKeys.
func (f *Fake) DeleteImage(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("DeleteImage", key); err != nil {
		return err
	}
	f.ImageKeys = slices.DeleteFunc(f.ImageKeys, func(k string) bool { return k == key })
	return nil
}

// CreateVM records the call and adds a stopped instance.
func (f *Fake) CreateVM(_ context.Context, image string, spec backend.VMSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("CreateVM", image, spec); err != nil {
		return err
	}
	if f.Instances == nil {
		f.Instances = map[string]backend.Instance{}
	}
	f.Instances[spec.Name] = backend.Instance{
		Name:            spec.Name,
		Status:          backend.StatusStopped,
		Size:            spec.Size,
		AgentDirs:       slices.Clone(spec.AgentDirs),
		AgentForwarding: spec.AgentForwarding,
	}
	return nil
}

// Start records the call and marks the instance running.
func (f *Fake) Start(_ context.Context, vm string, c backend.Confinement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Start", vm, c); err != nil {
		return err
	}
	return f.change(vm, func(in *backend.Instance) { in.Status = backend.StatusRunning })
}

// Confined records the call and answers !Unconfined.
func (f *Fake) Confined(_ context.Context, vm string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Confined", vm); err != nil {
		return false, err
	}
	return !f.Unconfined, nil
}

// Stop records the call and marks the instance stopped.
func (f *Fake) Stop(_ context.Context, vm string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Stop", vm); err != nil {
		return err
	}
	return f.change(vm, func(in *backend.Instance) { in.Status = backend.StatusStopped })
}

// Delete records the call and removes the instance.
func (f *Fake) Delete(_ context.Context, vm string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Delete", vm); err != nil {
		return err
	}
	delete(f.Instances, vm)
	return nil
}

// List records the call and returns Instances, sorted by name.
func (f *Fake) List(context.Context) ([]backend.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("List"); err != nil {
		return nil, err
	}
	var ins []backend.Instance
	for _, name := range slices.Sorted(maps.Keys(f.Instances)) {
		ins = append(ins, f.Instances[name])
	}
	return ins, nil
}

// Inspect records the call and returns the instance.
func (f *Fake) Inspect(_ context.Context, vm string) (backend.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Inspect", vm); err != nil {
		return backend.Instance{}, err
	}
	in, ok := f.Instances[vm]
	if !ok {
		return backend.Instance{}, fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	}
	return in, nil
}

// Exec records the call.
func (f *Fake) Exec(_ context.Context, vm string, argv []string, o backend.ExecOpts) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.record("Exec", vm, slices.Clone(argv), o)
}

// Shell records the call and returns ShellCmd.
func (f *Fake) Shell(_ context.Context, vm, dir string) (backend.Command, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Shell", vm, dir); err != nil {
		return backend.Command{}, err
	}
	return f.ShellCmd, nil
}

// SSH records the call and returns Target.
func (f *Fake) SSH(_ context.Context, vm string) (backend.SSHTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("SSH", vm); err != nil {
		return backend.SSHTarget{}, err
	}
	return f.Target, nil
}

// SetSize records the call and sets the instance's size where s is set.
func (f *Fake) SetSize(_ context.Context, vm string, s backend.Size) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("SetSize", vm, s); err != nil {
		return err
	}
	return f.change(vm, func(in *backend.Instance) {
		if s.CPUs > 0 {
			in.Size.CPUs = s.CPUs
		}
		if s.MemoryGiB > 0 {
			in.Size.MemoryGiB = s.MemoryGiB
		}
		if s.DiskGiB > 0 {
			in.Size.DiskGiB = s.DiskGiB
		}
	})
}

// SetPorts records the call.
func (f *Fake) SetPorts(_ context.Context, vm string, p backend.Ports) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("SetPorts", vm, p); err != nil {
		return err
	}
	return f.change(vm, func(*backend.Instance) {})
}

// SetAgentDirs records the call and sets the instance's agent folders.
func (f *Fake) SetAgentDirs(_ context.Context, vm string, dirs []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("SetAgentDirs", vm, slices.Clone(dirs)); err != nil {
		return err
	}
	return f.change(vm, func(in *backend.Instance) { in.AgentDirs = slices.Clone(dirs) })
}

// SetAgentForwarding records the call and sets the instance's forwarding.
func (f *Fake) SetAgentForwarding(_ context.Context, vm string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("SetAgentForwarding", vm, on); err != nil {
		return err
	}
	return f.change(vm, func(in *backend.Instance) { in.AgentForwarding = on })
}
