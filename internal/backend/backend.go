// Package backend is what runs devvm's VMs: one interface every backend
// implements (Lima, smolvm, Docker), the features each one has (Caps), the
// registry of backends and the choice of backend for a VM.
//
// A backend only runs what it's told. What a VM is made of (its image, its
// mounts, its size, its network policy) is decided outside, and anything
// that doesn't depend on the backend (.git protection, the network policy
// proxy) stays outside too. Before asking a backend for something, callers
// check it has the features that needs with Require.
package backend

import (
	"context"
	"errors"
	"io"

	"github.com/w3cj/devvm/internal/msg"
)

// Backend runs VMs and builds their images. Methods that change a VM's
// config (CreateVM's spec, the Set methods) take effect at its next start;
// the Set methods need the VM stopped. Errors are messages (msg.M) where
// the user can act on them.
type Backend interface {
	// Name is the backend's name, as --backend and VM state have it: lima.
	Name() string
	// Caps says which features the backend has, and how.
	Caps() Caps
	// Check reports whether the backend can run here: installed, a
	// version devvm supports, a host it supports. Its error is the first
	// of Problems.
	Check(ctx context.Context) error
	// Problems lists everything the backend needs that this host lacks;
	// none when it can run here.
	Problems(ctx context.Context) []Problem
	// NameMax is the longest VM name the backend takes; 0 for no limit.
	// Callers shorten the names they make to fit.
	NameMax() int

	// BuildImage builds an image from its parent (or, for base, from the
	// backend's own Ubuntu or spec.From) by running its scripts in order.
	BuildImage(ctx context.Context, spec ImageSpec) (Built, error)
	// Images lists the keys of the images the backend holds.
	Images(ctx context.Context) ([]string, error)
	// DeleteImage deletes an image. A missing one is not an error.
	DeleteImage(ctx context.Context, key string) error
	// BuildTools runs the shared tools' build script in a throwaway VM
	// made from base's image, with spec.Out mounted writable, and deletes
	// the VM: at once, or after a failure once the run ends (a ui
	// cleanup), so the run log ends with what failed.
	BuildTools(ctx context.Context, spec ToolsSpec) error

	// CreateVM makes a VM from an image, stopped.
	CreateVM(ctx context.Context, image string, spec VMSpec) error
	// Start starts a VM confined as asked, and returns once it answers.
	// A running VM is left running; its confinement is checked with
	// Confined.
	Start(ctx context.Context, vm string, c Confinement) error
	// Confined reports whether a running VM still runs under the
	// confinement Start gave it (not started some other way since).
	Confined(ctx context.Context, vm string) (bool, error)
	// Lifeline is the pid file of the host process that runs the VM once
	// it starts, which the VM's proxy follows: it exits when that process
	// does. Not a VM's own file: the VM can't write it.
	Lifeline(vm string) string
	// ProxyURL is devvm's proxy as a VM started with c reaches it, its
	// user and password included; "" unless c.Net is NetProxy.
	ProxyURL(c Confinement) string
	// Stop stops a VM. A stopped one is left as it is.
	Stop(ctx context.Context, vm string) error
	// Delete deletes a VM, stopping it first.
	Delete(ctx context.Context, vm string) error
	// List lists the backend's VMs, without its images.
	List(ctx context.Context) ([]Instance, error)
	// Inspect returns one VM, or an error matching ErrNotFound.
	Inspect(ctx context.Context, vm string) (Instance, error)

	// Exec runs argv in a running VM and waits for it.
	Exec(ctx context.Context, vm string, argv []string, o ExecOpts) error
	// Shell returns the command that opens an interactive shell in a
	// running VM, in dir. The caller runs it in place of devvm.
	Shell(ctx context.Context, vm, dir string) (Command, error)
	// SSH says how ssh and editors reach a running VM.
	SSH(ctx context.Context, vm string) (SSHTarget, error)

	// SetSize changes a stopped VM's size; zero fields are left as they are.
	SetSize(ctx context.Context, vm string, s Size) error
	// SetPorts changes which of a stopped VM's ports are forwarded.
	SetPorts(ctx context.Context, vm string, p Ports) error
	// SetAgentDirs replaces a stopped VM's agent folder mounts.
	SetAgentDirs(ctx context.Context, vm string, dirs []string) error
	// SetAgentForwarding turns a stopped VM's ssh-agent forwarding on or off.
	SetAgentForwarding(ctx context.Context, vm string, on bool) error
}

// Problem is something a backend needs that the host lacks, as devvm
// doctor reports it.
type Problem struct {
	// Key says what it is, for scripts that offer fixes: lima, qemu,
	// kvm, kvm-access…
	Key string
	// Fix is the command that fixes it, its words separated by spaces
	// (no shell syntax); "" when there's none to run.
	Fix string
	// Msg says what's missing and what to do.
	Msg msg.M
}

// ErrNotFound is the error, or wraps it, for a VM the backend doesn't have.
var ErrNotFound = errors.New("no such VM")

// LoggedError is a failure whose details are in the run log, from a
// command run with ui.Logged: callers end with ui.DieLogged(e.Msg), which
// shows the log's end, rather than ui.Die.
type LoggedError struct{ Msg msg.M }

// Logged returns m as a LoggedError.
func Logged(m msg.M) error { return &LoggedError{Msg: m} }

func (e *LoggedError) Error() string { return e.Msg.Error() }

// Unwrap returns the message, so errors.As finds it.
func (e *LoggedError) Unwrap() error { return e.Msg }

// ImageSpec is an image to build.
type ImageSpec struct {
	Image   string   // its name: base, or base's layers joined with + (node+claude)
	Key     string   // its key (image.Key); the backend stores it under it
	Parent  string   // its parent's key; "" for base
	From    *From    // base only: the container image it starts from; nil for Ubuntu
	Scripts []Script // run in order as the VM's user, who has sudo: the layer's, then base's features

	MemoryGiB float64 // memory while building; 0 for the backend's default
	Net       Net     // the network the build gets: NetInternet or NetNone
}

// From is the container image base starts from (--from).
type From struct {
	Ref    string // as given: ghcr.io/org/image:tag
	Digest string // the digest devvm recorded for it (sha256:…), which is what's built
}

// Script is a script run in a VM, fed on its standard input to bash.
type Script struct {
	Name string // for status lines and errors: base.sh
	Path string // on the host
}

// ToolsSpec is a shared tools build: Script, fed to bash with Args, in a
// VM made from base's image, which mounts nothing from the host but Out.
type ToolsSpec struct {
	Base   string // base's image key
	Out    string // a host folder of the build's own, writable in the VM at OutAt
	OutAt  string
	Script Script
	Args   []string
	Net    Net // the network the build gets: NetInternet or NetNone
}

// Built is what a build reports, for the image's record.
type Built struct {
	Origin string // what base started from: the Ubuntu image (URL sha256:DIGEST) or From's digest; "" above base
	Libc   string // base's C library: gnu, or musl (Alpine); "" above base
}

// VMSpec is a VM to create from an image. Mounts are at the same path in
// the VM as on the host, except the tools share.
type VMSpec struct {
	Name      string
	Project   string   // the project folder, read-write; "" for none (--clone)
	AgentDirs []string // agents' host folders, read-only
	Tools     string   // the shared tools folder, read-only at ToolsAt; "" for none
	ToolsAt   string   // where the tools folder is in the VM

	AgentForwarding bool  // forward ssh-agent into the VM
	Size            Size  // zero fields take the image's
	Ports           Ports // which ports to forward to host loopback
}

// Size is a VM's CPUs, memory and disk. Zero is unset.
type Size struct {
	CPUs      int
	MemoryGiB float64
	DiskGiB   float64
}

// Ports is which guest ports a VM forwards to the same port on host
// loopback: all of them, or those in Ranges (none when it's empty).
type Ports struct {
	All    bool
	Ranges []PortRange
}

// PortRange is a range of ports, both ends included.
type PortRange struct{ From, To int }

// Net is the network a VM starts with.
type Net int

const (
	// NetDirect is the backend's own network: the internet and host
	// loopback, unfiltered. Project VMs without network policy.
	NetDirect Net = iota
	// NetProxy reaches nothing but devvm's proxy on host loopback, which
	// the guest's proxy settings name. Project VMs with network policy.
	NetProxy
	// NetInternet reaches the internet but not host loopback, and has no
	// port forwards. Image and tool builds.
	NetInternet
	// NetNone reaches nothing. Builds that need no network.
	NetNone
)

// Confinement is how a VM is started.
type Confinement struct {
	Net Net
	// NetProxy: the port devvm's proxy listens on (host 127.0.0.1), and the
	// user and password the guest's proxy settings carry.
	ProxyPort     int
	ProxyUser     string
	ProxyPassword string
	// Env is more guest environment, for every login (the proxy's CA
	// files, NO_PROXY).
	Env map[string]string
	// SSHPort is the host port the VM's ssh listens on; 0 lets the
	// backend choose.
	SSHPort int
	// AgentSocket is the ssh-agent the VM's forwarding uses: "" for the
	// user's own (SSH_AUTH_SOCK), or devvm's.
	AgentSocket string
}

// Status is a VM's state as its backend reports it.
type Status string

// Statuses. A backend reports anything else (broken, starting) as it has it.
const (
	StatusRunning Status = "Running"
	StatusStopped Status = "Stopped"
)

// Instance is a VM as its backend has it.
type Instance struct {
	Name            string
	Status          Status
	Size            Size     // what it has, all set
	AgentDirs       []string // its agent folder mounts
	AgentForwarding bool
}

// ExecOpts is where a command run in a VM runs and what it's connected to.
// Nil streams are empty input and discarded output.
type ExecOpts struct {
	Dir    string // the folder in the VM; "" for /
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Command is a host command to run: Path with Args (Args[0] included), in
// Dir, with Env as its whole environment.
type Command struct {
	Path string
	Args []string
	Env  []string
	Dir  string
}

// SSHTarget is how ssh reaches a VM.
type SSHTarget struct {
	Host string
	Port int
	User string
	// Options are the other ssh options that log in (IdentityFile,
	// IdentitiesOnly…), in order, values as an ssh config has them.
	// Nothing that turns off host key checks or changes what ssh runs.
	Options []SSHOption
	// ConfigFile is an ssh config the backend keeps for the VM, with its
	// identity; "" when it has none.
	ConfigFile string
}

// SSHOption is an ssh config option: IdentityFile "/path".
type SSHOption struct{ Key, Value string }
