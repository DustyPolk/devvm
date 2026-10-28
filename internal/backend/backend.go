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
	// ImageSize is the disk space an image takes on this machine, in
	// bytes; blocks it shares with other images or VMs count too. A
	// missing one is an error matching fs.ErrNotExist.
	ImageSize(ctx context.Context, key string) (int64, error)
	// DeleteImage deletes an image. A missing one is not an error.
	DeleteImage(ctx context.Context, key string) error
	// BuildTools runs the shared tools' build script in a throwaway VM
	// made from base's image, with spec.Out mounted writable, and deletes
	// the VM: at once, or after a failure once the run ends (a ui
	// cleanup), so the run log ends with what failed. The images it
	// leaves are in spec.Format, which is ToolFormat's.
	BuildTools(ctx context.Context, spec ToolsSpec) error
	// ToolFormat is the format of the tool images the backend's VMs
	// mount, which their keys hash: "" for the build script's own
	// (compressed EROFS), else a name of the backend's that changes
	// whenever what it makes of them does.
	ToolFormat() string

	// CreateVM makes a VM from an image, stopped.
	CreateVM(ctx context.Context, image string, spec VMSpec) error
	// Start starts a VM confined as asked, and returns once it answers.
	// A running VM is left running; its confinement is checked with
	// Confined.
	Start(ctx context.Context, vm string, c Confinement) error
	// Confined reports whether a running VM still runs confined to
	// devvm's proxy: started by Start with NetProxy, not some other way
	// since.
	Confined(ctx context.Context, vm string) (bool, error)
	// Lifeline is the pid file of the host process that runs the VM once
	// it starts, which the VM's proxy follows: it exits when that process
	// does. Not a VM's own file: the VM can't write it.
	Lifeline(ctx context.Context, vm string) string
	// StartID identifies a running VM's current start: every start gets
	// another, whether devvm or something else made it. Asked only of a
	// running VM; "" when the backend can't tell.
	StartID(ctx context.Context, vm string) (string, error)
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
	// SetAgentForwarding turns a stopped VM's ssh-agent forwarding on or
	// off (FeatureAgentToggle).
	SetAgentForwarding(ctx context.Context, vm string, on bool) error
}

// KeyFiles is a backend that puts files of its own into base (scripts it
// runs before devvm's, the image it starts from), which base's key hashes
// too: by name, their contents. A backend that puts nothing in doesn't implement it.
type KeyFiles interface {
	KeyFiles() map[string][]byte
}

// Decider is a backend that confines a VM with network policy by asking
// devvm's proxy about each connection the VM opens (smolvm's mediated
// egress): the proxy's decider takes over only those to the proxy's address
// in the VM (ProxyURL's host and port) and refuses the rest. Start with
// NetProxy then needs Confinement's Decider and DeciderToken. A backend that
// confines VMs some other way doesn't implement it.
type Decider interface {
	UsesDecider() bool
}

// UsesDecider reports whether b confines VMs through devvm's decider.
func UsesDecider(b Backend) bool {
	d, ok := b.(Decider)
	return ok && d.UsesDecider()
}

// ProxySocket is a backend whose VMs reach devvm's proxy through a unix
// socket on the host rather than its loopback port (Docker's, through a
// relay on the VM's own network): the proxy listens on that socket too.
// A backend whose VMs reach the port doesn't implement it.
type ProxySocket interface {
	// ProxySocket is the socket's path for the VM; "" for none.
	ProxySocket(vm string) string
}

// ProxySocketOf is the socket b's VM reaches devvm's proxy through
// (ProxySocket); "" on other backends.
func ProxySocketOf(b Backend, vm string) string {
	if s, ok := b.(ProxySocket); ok {
		return s.ProxySocket(vm)
	}
	return ""
}

// AgentStarter is a backend whose VM forwards whatever SSH agent the
// command that starts it names (smolvm's machine start), so a VM started
// some other way than by Start can have one devvm didn't choose for it. A
// backend whose agent is the VM's config, or fixed when it's made,
// doesn't implement it.
type AgentStarter interface {
	// AgentUnchosen reports whether a running VM may forward an agent
	// devvm didn't choose: made forwarding one, and not running on the
	// process Start last started.
	AgentUnchosen(ctx context.Context, vm string) (bool, error)
}

// AgentUnchosen reports whether b's running VM may forward an agent devvm
// didn't choose for it (AgentStarter); never on other backends.
func AgentUnchosen(ctx context.Context, b Backend, vm string) (bool, error) {
	a, ok := b.(AgentStarter)
	if !ok {
		return false, nil
	}
	return a.AgentUnchosen(ctx, vm)
}

// DiskUser is a backend whose VMs' disks have no size of their own
// (FeatureDisk none): it says how much one holds instead, which devvm size
// shows, and the size it's meant to stay under, past which launches warn.
type DiskUser interface {
	// DiskUsed is what the VM's disk holds, in bytes, and the size it's
	// meant to stay under.
	DiskUsed(ctx context.Context, vm string) (used, limit int64, err error)
}

// Reporter is a backend that tells devvm doctor more about what it runs
// on (Docker's engine, which folders it can mount), once Problems found
// none.
type Reporter interface {
	Report(ctx context.Context) []msg.M
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
// command run into it: the CLI ends with the UI's DieLogged(e.Msg), which
// shows the log's end, rather than Die.
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
	Data []byte
}

// ToolsSpec is a shared tools build: Script, fed to bash with Args, in a
// VM made from base's image, which mounts nothing from the host but Out.
type ToolsSpec struct {
	Base   string // base's image key
	Out    string // a host folder of the build's own, writable in the VM at OutAt
	OutAt  string
	Script Script
	Args   []string
	Format string // the images' format: the backend's ToolFormat
	Net    Net    // the network the build gets: NetInternet or NetNone
}

// Built is what a build reports, for the image's record.
type Built struct {
	Origin string // what base started from: the Ubuntu image (URL sha256:DIGEST) or From's digest; "" above base
	Libc   string // base's C library: gnu, or musl (Alpine); "" above base
}

// VMSpec is a VM to create from an image. Mounts are at the same path in
// the VM as on the host, except the tools share.
type VMSpec struct {
	Name    string
	Project string // the project folder, read-write; "" for none (--clone)
	// CloneAt is where a --clone VM keeps its own clone of the project (the
	// folder's path); "" for a VM that mounts the folder. A backend may keep
	// it on a disk of its own.
	CloneAt   string
	AgentDirs []string // agents' host folders, read-only
	Tools     string   // the shared tools folder, read-only at ToolsAt; "" for none
	ToolsAt   string   // where the tools folder is in the VM

	AgentForwarding bool // forward ssh-agent into the VM
	// AgentToggle is whether its forwarding may be turned on or off later
	// (FeatureAgentToggle let through): a backend that sets forwarding
	// only when a VM is made makes it ready for that.
	AgentToggle bool
	Size        Size  // zero fields take the image's
	Ports       Ports // which ports to forward to host loopback
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
	// NetProxy on a backend with a decider (Decider): the proxy's decider,
	// on host loopback (127.0.0.1:PORT), and the token, 64 hex digits, the
	// backend proves itself to it with. Never in the VM.
	Decider      string
	DeciderToken string
	// Env is more guest environment, for every login (the proxy's CA
	// files, NO_PROXY).
	Env map[string]string
	// SSHPort is the host port the VM's ssh listens on; 0 lets the
	// backend choose.
	SSHPort int
	// SSH is whether the VM gets ssh for editors from this start
	// (FeatureSSH usable): a backend that emulates it runs it only then,
	// one with its own always has it.
	SSH bool
	// AgentSocket is the ssh-agent the VM's forwarding uses: "" for the
	// user's own (SSH_AUTH_SOCK), or devvm's.
	AgentSocket string
	// NoAgent is set when the VM's choice is no agent: a backend whose VM
	// forwards one anyway (made so, for agent-toggle) gives it none.
	NoAgent bool
	// Ports is what a VM that lists its ports forwards from this start:
	// the list, less the ports a program on the host has. The backend
	// forwards these where its VM forwards others. nil leaves its ports as
	// they are (every port, none, a build's).
	Ports *Ports
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
	// AgentFixed is set when its forwarding can't be turned on: it was
	// made without, on a backend that sets forwarding only then.
	AgentFixed bool
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
	// ProxyCommand is the command ssh connects through instead of Host and
	// Port (ssh's ProxyCommand), a line for the user's shell, its words
	// quoted, no % tokens; "" for none.
	ProxyCommand string
}

// SSHOption is an ssh config option: IdentityFile "/path".
type SSHOption struct{ Key, Value string }
