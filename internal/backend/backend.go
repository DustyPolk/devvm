// Package backend is what runs devvm's VMs: one interface every backend
// implements (Lima's), the features each one has (Caps), the
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
	"strconv"

	"github.com/w3cj/devvm/internal/msg"
)

// Backend runs VMs and builds their images. A VM is made as its spec
// wants (CreateVM), and each start of a stopped VM makes it what the
// caller wants then (Start's Want), changing only what differs: nothing
// changes a stopped VM's config in between. Errors are messages (msg.M)
// where the user can act on them.
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

	// CreateVM makes a VM from an image, stopped, as spec wants it.
	CreateVM(ctx context.Context, image string, spec VMSpec) error
	// Start starts a stopped VM as w wants it, confined as c says, and
	// returns once it answers: it first changes what of w and c the VM's
	// config doesn't have yet, and nothing else, after checking w with
	// Validate. Its ports are forwarded as w says once it runs. A
	// running VM is left as it is, w and c unapplied (callers say
	// what changes at its next start); its confinement is checked with
	// Confined.
	Start(ctx context.Context, vm string, w Want, c Confinement) error
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

	// SetPorts changes which of a running VM's ports are forwarded, at
	// once: its forwarder started again with p as Start starts it with c
	// (HostKey, ProxyPort, Free) while the VM runs on. Connections open
	// through its old forwarder end. A stopped VM is left as it is: its
	// next Start's Want has its ports.
	SetPorts(ctx context.Context, vm string, p Ports, c Confinement) error
}

// KeyFiles is a backend that puts files of its own into base (scripts it
// runs before devvm's, the image it starts from), which base's key hashes
// too: by name, their contents. A backend that puts nothing in doesn't implement it.
type KeyFiles interface {
	KeyFiles() map[string][]byte
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
	Image   string   // its name: base, or base's layers joined with + (docker+foo)
	Key     string   // its key (image.Key); the backend stores it under it
	Parent  string   // its parent's key; "" for base
	From    *From    // base only: the container image it starts from; nil for Ubuntu
	Scripts []Script // run in order as the VM's user, who has sudo: the layer's, then base's features

	MemoryGiB float64 // memory while building; 0 for the backend's default
	Net       Net     // the network the build gets: NetInternet or NetNone
	// Trust is the user's CAs for the build VM: added before anything in
	// it downloads, base's preparation included, and removed once its
	// scripts are done, before it stops.
	Trust Trust
}

// Trust is how a build VM trusts the user's own CAs while it builds: Add
// installs them and Remove takes them out again, so the image doesn't
// keep them; both are run as root, fed to sh. Empty scripts for none.
type Trust struct{ Add, Remove Script }

// None reports whether there are no CAs to trust.
func (t Trust) None() bool { return len(t.Add.Data) == 0 }

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
	// Trust is the user's CAs for the build VM: added before the script
	// runs (Remove isn't needed: the VM is deleted, only Out is kept).
	Trust Trust
}

// Built is what a build reports, for the image's record and the build's
// output.
type Built struct {
	Origin string // what base started from: the Ubuntu image (URL sha256:DIGEST) or From's digest; "" above base
	Libc   string // base's C library: gnu, or musl (Alpine); "" above base
	// Fitted is base's CPUs and memory when fitting them to the host made
	// either less than base.yaml's (Fitted); zero when it didn't, and
	// above base.
	Fitted Size
}

// Want is what of a VM can change after it's made: CreateVM makes it so,
// and each Start of the stopped VM makes it so again where it differs.
// Agent folders are mounted at the same path in the VM as on the host.
type Want struct {
	// Size is its CPUs, memory and disk: zero fields are left as they
	// are (the image's, when it's made). A disk only grows: a smaller
	// one is refused (Validate).
	Size      Size
	AgentDirs []string // agents' host folders, read-only
	// AgentForwarding is whether the VM forwards an ssh-agent; changing
	// it once the VM is made needs FeatureAgentToggle.
	AgentForwarding bool
	Ports           Ports // which ports to forward to host loopback
}

// Validate returns the error a start of the VM in, stopped, would refuse
// w with, before it changes anything: a disk smaller than its own
// (disk_shrink).
func Validate(w Want, in Instance) error {
	if w.Size.DiskGiB > 0 && w.Size.DiskGiB < in.Size.DiskGiB {
		return msg.DiskShrink(gibText(in.Size.DiskGiB), in.Name, gibText(w.Size.DiskGiB))
	}
	return nil
}

// gibText is a size in GiB as messages show it: 100, 6.5.
func gibText(g float64) string { return strconv.FormatFloat(g, 'g', -1, 64) }

// VMSpec is a VM to create from an image, as Want wants it. Mounts are
// at the same path in the VM as on the host, except the tools share.
type VMSpec struct {
	Want
	Name    string
	Project string // the project folder, read-write; "" for none (--clone)
	// CloneAt is where a --clone VM keeps its own clone of the project (the
	// folder's path); "" for a VM that mounts the folder. A backend may keep
	// it on a disk of its own.
	CloneAt string
	Tools   string // the shared tools folder, read-only at ToolsAt; "" for none
	ToolsAt string // where the tools folder is in the VM

	// AgentToggle is whether its forwarding may be turned on or off later
	// (FeatureAgentToggle let through): a backend that sets forwarding
	// only when a VM is made makes it ready for that.
	AgentToggle bool
	// SSHPort is the host port the VM's ssh is forwarded from, as devvm
	// records it (Confinement.SSHPort at each start); 0 for none yet.
	SSHPort int
	// Env is the guest environment a start without network policy gives
	// it (Confinement.Env): a backend that keeps it in the VM's config
	// sets it at create, so the first start changes nothing.
	Env map[string]string
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
	// password the guest's proxy settings carry, with ProxyUser (ProxyURL).
	ProxyPort     int
	ProxyPassword string
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
	// Free returns the listed ports of a VM that lists its ports less
	// those a program on the host has, warning about those: what its
	// forwarder forwards, asked once any forwarder it had has stopped (its
	// own ports would read as taken). nil forwards every listed port.
	Free func(ctx context.Context, listed []PortRange) []PortRange
	// HostKey returns the VM's host key (TYPE BASE64) as devvm pins it
	// for its ssh config, read once it runs: a backend whose own ssh
	// devvm pins (Lima's) gives it to its port forwarder. nil for a
	// build.
	HostKey func(ctx context.Context) (string, error)
}

// Forwarded returns what the forwarder of a VM that forwards p forwards
// with c: nil for every port, else p's listed ports less those Free leaves
// out, empty for none.
func (c Confinement) Forwarded(ctx context.Context, p Ports) []PortRange {
	if p.All {
		return nil
	}
	rs := append([]PortRange{}, p.Ranges...)
	if c.Free != nil && len(rs) > 0 {
		rs = append([]PortRange{}, c.Free(ctx, rs)...)
	}
	return rs
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
}

// SSHOption is an ssh config option: IdentityFile "/path".
type SSHOption struct{ Key, Value string }
