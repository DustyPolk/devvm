package vmssh

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/daemon"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
	"github.com/w3cj/devvm/internal/vmname"
)

// Forwards starts and stops a backend's VMs' port forwarders (Forwarder):
// its hidden command, detached, one per VM that forwards its ports
// through one.
type Forwards struct {
	// Dirs are devvm's folders: each VM's forwarder's files (its pid, its
	// log, the host key it pins) are in the VM's backend folder
	// (paths.VM.Forwarder), and the masters' control sockets in the run
	// folder, named Prefix-<hash>.ctl, short for a socket's path.
	Dirs   paths.Dirs
	Prefix string
	// Run is where the control sockets go instead, when set: tests' run
	// folders can be too long for a socket.
	Run string
	// Cmd is devvm's hidden command that runs a forwarder
	// (_lima-ports), and Self is devvm's binary, which runs it.
	Cmd, Self string
	// Detach starts a process that outlives devvm (daemon.Detach).
	Detach daemon.Starter
	// Args returns a running process's arguments (platform.ProcessArgs).
	Args func(pid int) ([]string, error)
	// Tick is how often a start looks for the forwarder to be ready, 20
	// times at most.
	Tick time.Duration
}

// Spec is what a VM's forwarder needs: the VM's user and key, its host key
// (TYPE BASE64), the host ports it never forwards, the ports it forwards
// (nil for every one), and the backend's own flags (how it reaches the VM,
// how it knows the VM runs).
type Spec struct {
	User, Key, HostKey string
	Reserve            []int
	Only               []backend.PortRange
	Args               []string
}

// file is one of the forwarder's files.
func (f Forwards) file(vm, ext string) string { return f.Dirs.VM(vm).Forwarder(ext) }

// Log is the VM's forwarder's log.
func (f Forwards) Log(vm string) string { return f.file(vm, ".log") }

// KnownHosts is the host key the VM's forwarder pins.
func (f Forwards) KnownHosts(vm string) string { return f.file(vm, ".known_hosts") }

// Control is the master's control socket: in the run folder, named by a
// hash, as a socket's path is at most paths.SocketMax characters and ssh
// adds 17 while it makes it.
func (f Forwards) Control(vm string) string {
	sum := sha256.Sum256([]byte(vm))
	return filepath.Join(cmp.Or(f.Run, f.Dirs.Run()), f.Prefix+"-"+hex.EncodeToString(sum[:])[:16]+".ctl")
}

// logMax is the most a forwarder's log keeps from earlier starts: past
// it, a start empties it.
const logMax = 1 << 20

// ControlMax is the longest control socket path ssh can make.
const ControlMax = paths.SocketMax - 1 - 17

// Start starts the VM's forwarder, after any it had, and waits for it to
// say it forwards, in its status file, while it runs. One that doesn't is
// left to keep trying, and its log named in the error.
func (f Forwards) Start(vm string, spec Spec) error {
	f.Stop(vm)
	ctl := f.Control(vm)
	if len(ctl) > ControlMax {
		return fmt.Errorf("%s: its path is too long for a socket", ctl)
	}
	if err := os.MkdirAll(filepath.Dir(ctl), 0o700); err != nil {
		return err
	}
	kh := f.KnownHosts(vm)
	if err := state.WriteFile(kh, []byte(vm+" "+spec.HostKey+"\n")); err != nil {
		return err
	}
	out, err := os.OpenFile(f.Log(vm), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	fi, err := out.Stat()
	if err != nil {
		return err
	}
	if fi.Size() > logMax {
		if err := out.Truncate(0); err != nil {
			return err
		}
	}
	var reserve []string
	for _, p := range spec.Reserve {
		if p > 0 {
			reserve = append(reserve, strconv.Itoa(p))
		}
	}
	args := slices.Concat([]string{f.Cmd, "-vm", vm}, spec.Args, []string{
		"-user", spec.User, "-key", spec.Key, "-known-hosts", kh, "-control", ctl, "-reserve", strings.Join(reserve, ","),
	})
	if spec.Only != nil {
		args = append(args, "-only", backend.FormatPortRanges(spec.Only))
	}
	status := f.file(vm, ".status")
	pid, err := f.proc(vm).Start(f.Detach, daemon.Cmd{Path: f.Self, Args: args, Out: out, Status: status})
	if err != nil {
		return err
	}
	if _, ok := daemon.WaitReady(status, pid, func() bool { return daemon.Alive(pid) }, f.Tick, 20); ok {
		return nil
	}
	return fmt.Errorf("%s: its forwarder isn't forwarding", vm)
}

// proc is the VM's forwarder as a background process: its pid file in the
// VM's backend folder, and devvm's hidden command (Cmd) with -vm VM in its
// arguments.
func (f Forwards) proc(vm string) daemon.Proc {
	return daemon.Proc{
		PIDFile: f.file(vm, ".pid"), Args: f.Args,
		Is: func(args []string) bool { return daemon.HasFlag(args, f.Cmd, "-vm", vm) },
	}
}

// Running reports whether the VM's forwarder runs.
func (f Forwards) Running(vm string) bool { return f.proc(vm).Running() }

// Lock holds the VM's forwarder's lock until unlock is called: whatever
// stops its forwarder, asks which ports are free and starts the next one
// holds it throughout, so two at once (a start and devvm ports, or two
// devvm ports) never leave a forwarder whose pid file names another, which
// nothing would stop. Another devvm holding it is waited for, until ctx
// ends.
func (f Forwards) Lock(ctx context.Context, vm string) (unlock func(), err error) {
	if err := state.PrivateDir(f.Dirs.VM(vm).Backend()); err != nil {
		return nil, err
	}
	return platform.LockFile(ctx, f.file(vm, ".lock"))
}

// Stop stops the VM's forwarder, if it runs, waiting up to two seconds for
// it to end, then killing it and waiting up to two more: a start that
// follows finds its ports free.
func (f Forwards) Stop(vm string) { f.proc(vm).Stop(2 * time.Second) }

// Forget stops the VM's forwarder and deletes the VM's backend folder:
// the forwarder's files, and the backend's own beside them (its record,
// keys). A build VM (not a project VM: an image's, the tools build's)
// loses its whole folder; the rest of a project VM's is its teardown's.
// Every backend's VMs' files go this way, the forwarder stopped first, so
// none outlives its pid file.
func (f Forwards) Forget(vm string) error {
	f.Stop(vm)
	if !vmname.IsVM(vm) {
		return os.RemoveAll(f.Dirs.VM(vm).Dir)
	}
	return os.RemoveAll(f.Dirs.VM(vm).Backend())
}
