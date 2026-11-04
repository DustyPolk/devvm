// Package backendtest has a fake backend for tests: it records every call
// and answers from what the test preset.
package backendtest

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/state"
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
	ImageSizes   map[string]int64      // ImageSize's answers, by key
	Built        backend.Built         // BuildImage's answer
	Target       backend.SSHTarget     // SSH's answer
	Forward      backend.ForwardTarget // ForwardTarget's answer, for a running VM
	ShellCmd     backend.Command       // Shell's answer
	Unconfined   bool                  // Confined answers false (else: true, or for a VM Start started, whether with NetProxy)
	LifelineFile string                // Lifeline's answer
	Missing      []backend.Problem     // Problems' answer
	Format       string                // ToolFormat's answer
	// ExecFunc, when set, is what Exec returns after recording the call
	// (Errs aside).
	ExecFunc func(vm string, argv []string, o backend.ExecOpts) error
	Errs     map[string]error // by method name: returned in place of its answer, changing nothing
	// ToolsBuild, when set, is what BuildTools does after recording the
	// call, in place of writing the images.
	ToolsBuild func(backend.ToolsSpec) error

	mu     sync.Mutex
	calls  []Call
	nets   map[string]backend.Net // by name: the network Start last started it with
	starts map[string]int         // by name: how many starts it had
}

var _ backend.Backend = (*Fake)(nil)

// New returns a fake named name with these capabilities, listing every
// feature caps leaves out as None, as a backend's Caps list every one.
func New(name string, caps backend.Caps) *Fake {
	all := backend.Caps{}
	for _, f := range backend.Features {
		all[f] = caps[f]
	}
	maps.Copy(all, caps)
	return &Fake{BackendName: name, Capabilities: all, Instances: map[string]backend.Instance{}}
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

// Problems records the call and returns Missing.
func (f *Fake) Problems(context.Context) []backend.Problem {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.record("Problems")
	return slices.Clone(f.Missing)
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

// ImageSize records the call and returns ImageSizes' size for the key, an
// error matching fs.ErrNotExist for a key ImageKeys lacks.
func (f *Fake) ImageSize(_ context.Context, key string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ImageSize", key); err != nil {
		return 0, err
	}
	if !slices.Contains(f.ImageKeys, key) {
		return 0, fmt.Errorf("image %s: %w", key, fs.ErrNotExist)
	}
	return f.ImageSizes[key], nil
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

// BuildTools records the call, then runs ToolsBuild on the spec, or
// writes each image its script would (every third argument, from the
// third) into spec.Out as "image FILE".
func (f *Fake) BuildTools(_ context.Context, spec backend.ToolsSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("BuildTools", spec); err != nil {
		return err
	}
	if f.ToolsBuild != nil {
		return f.ToolsBuild(spec)
	}
	for i := 2; i < len(spec.Args); i += 3 {
		file := spec.Args[i]
		if err := state.WriteFile(filepath.Join(spec.Out, file), []byte("image "+file+"\n")); err != nil {
			return err
		}
	}
	return nil
}

// ToolFormat returns Format.
func (f *Fake) ToolFormat() string { return f.Format }

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
	in := backend.Instance{Name: spec.Name, Status: backend.StatusStopped}
	apply(&in, spec.Want)
	f.Instances[spec.Name] = in
	return nil
}

// Start records the call and marks the instance running, made what w
// wants first when it was stopped (backend.Validate's refusal returned).
func (f *Fake) Start(_ context.Context, vm string, w backend.Want, c backend.Confinement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Start", vm, w, c); err != nil {
		return err
	}
	if in, ok := f.Instances[vm]; ok && in.Status != backend.StatusRunning {
		if err := backend.Validate(w, in); err != nil {
			return err
		}
	}
	err := f.change(vm, func(in *backend.Instance) {
		if in.Status != backend.StatusRunning {
			apply(in, w)
		}
		in.Status = backend.StatusRunning
	})
	if err != nil {
		return err
	}
	if f.nets == nil {
		f.nets = map[string]backend.Net{}
	}
	f.nets[vm] = c.Net
	f.bump(vm)
	return nil
}

// Confined records the call and answers false with Unconfined; else, for
// a VM Start started, whether it started it with NetProxy, and true for
// one preset running.
func (f *Fake) Confined(_ context.Context, vm string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Confined", vm); err != nil {
		return false, err
	}
	if n, ok := f.nets[vm]; ok && !f.Unconfined {
		return n == backend.NetProxy, nil
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

// Rename records the call and gives the instance its new name; one that
// runs, or a name an instance has, is refused.
func (f *Fake) Rename(_ context.Context, vm, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("Rename", vm, to); err != nil {
		return err
	}
	in, ok := f.Instances[vm]
	switch _, taken := f.Instances[to]; {
	case !ok:
		return fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	case in.Status != backend.StatusStopped:
		return fmt.Errorf("%s runs", vm)
	case taken:
		return fmt.Errorf("%s exists", to)
	}
	delete(f.Instances, vm)
	in.Name = to
	f.Instances[to] = in
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
	if err := f.record("Exec", vm, slices.Clone(argv), o); err != nil || f.ExecFunc == nil {
		return err
	}
	return f.ExecFunc(vm, argv, o)
}

// Lifeline returns LifelineFile.
func (f *Fake) Lifeline(context.Context, string) string { return f.LifelineFile }

// StartID returns a running instance's start: "start-N" for its Nth by
// Start, "start-0" for one preset running. Restart (a start outside
// Start) gives it another.
func (f *Fake) StartID(_ context.Context, vm string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	in, ok := f.Instances[vm]
	if !ok {
		return "", fmt.Errorf("%s: %w", vm, backend.ErrNotFound)
	}
	if in.Status != backend.StatusRunning {
		return "", nil
	}
	return "start-" + strconv.Itoa(f.starts[vm]), nil
}

// Restart stands for a start of vm outside devvm: it runs, on a start of
// its own.
func (f *Fake) Restart(vm string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.change(vm, func(in *backend.Instance) { in.Status = backend.StatusRunning })
	f.bump(vm)
}

// bump gives vm's next start. The caller holds the lock.
func (f *Fake) bump(vm string) {
	if f.starts == nil {
		f.starts = map[string]int{}
	}
	f.starts[vm]++
}

// ProxyURL returns the proxy at proxy.test on c's port, with its user and
// password.
func (f *Fake) ProxyURL(c backend.Confinement) string {
	if c.Net != backend.NetProxy {
		return ""
	}
	return backend.ProxyURL("proxy.test", c.ProxyPort, c.ProxyPassword)
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

// apply makes an instance what w wants: its size where w sets it, its
// agent folders and forwarding.
func apply(in *backend.Instance, w backend.Want) {
	if w.Size.CPUs > 0 {
		in.Size.CPUs = w.Size.CPUs
	}
	if w.Size.MemoryGiB > 0 {
		in.Size.MemoryGiB = w.Size.MemoryGiB
	}
	if w.Size.DiskGiB > 0 {
		in.Size.DiskGiB = w.Size.DiskGiB
	}
	in.AgentDirs = slices.Clone(w.AgentDirs)
	in.AgentForwarding = w.AgentForwarding
}

// ForwardTarget records the call and returns Forward for a running
// instance.
func (f *Fake) ForwardTarget(_ context.Context, vm string) (backend.ForwardTarget, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("ForwardTarget", vm); err != nil {
		return backend.ForwardTarget{}, err
	}
	if in, ok := f.Instances[vm]; !ok || in.Status != backend.StatusRunning {
		return backend.ForwardTarget{}, fmt.Errorf("%s: not running", vm)
	}
	return f.Forward, nil
}
