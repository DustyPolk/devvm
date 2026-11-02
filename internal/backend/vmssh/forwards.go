package vmssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/state"
)

// Forwards starts and stops a backend's VMs' port forwarders (Forwarder):
// its hidden command, detached, one per VM that forwards its ports
// through one.
type Forwards struct {
	// Dir is the folder of each VM's forwarder's files (its pid, its log,
	// the host key it pins), beside the VMs' records.
	Dir string
	// Run is the folder of the masters' control sockets, which are named
	// Prefix-<hash>.ctl.
	Run, Prefix string
	// Cmd is devvm's hidden command that runs a forwarder
	// (_smolvm-ports), and Self is devvm's binary, which runs it.
	Cmd, Self string
	// Detach starts a process that outlives devvm (platform.StartDetached).
	Detach func(path string, args []string, out *os.File) (int, error)
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
func (f Forwards) file(vm, ext string) string { return filepath.Join(f.Dir, vm+ext) }

// Log is the VM's forwarder's log.
func (f Forwards) Log(vm string) string { return f.file(vm, ".ports.log") }

// KnownHosts is the host key the VM's forwarder pins.
func (f Forwards) KnownHosts(vm string) string { return f.file(vm, ".ports.known_hosts") }

// Control is the master's control socket: short, as a socket's path is at
// most 104 characters and ssh adds 17 while it makes it.
func (f Forwards) Control(vm string) string {
	sum := sha256.Sum256([]byte(vm))
	return filepath.Join(f.Run, f.Prefix+"-"+hex.EncodeToString(sum[:])[:16]+".ctl")
}

// logMax is the most a forwarder's log keeps from earlier starts: past
// it, a start empties it.
const logMax = 1 << 20

// controlMax is the longest control socket path ssh can make.
const controlMax = 104 - 1 - 17

// Start starts the VM's forwarder, after any it had, and waits for it to
// say it forwards. One that doesn't is left to keep trying, and its log
// named in the error.
func (f Forwards) Start(vm string, spec Spec) error {
	f.Stop(vm)
	ctl := f.Control(vm)
	if len(ctl) > controlMax {
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
	before := fi.Size()
	if before > logMax {
		if err := out.Truncate(0); err != nil {
			return err
		}
		before = 0
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
	pid, err := f.Detach(f.Self, args, out)
	if err != nil {
		return err
	}
	if err := state.WriteFile(f.file(vm, ".ports.pid"), []byte(strconv.Itoa(pid)+"\n")); err != nil {
		return err
	}
	want := []byte("forwarding " + vm + "'s ports")
	for range 20 {
		if b, err := os.ReadFile(f.Log(vm)); err == nil && int64(len(b)) >= before && bytes.Contains(b[before:], want) {
			return nil
		}
		if _, ok := f.pid(vm); !ok {
			break
		}
		time.Sleep(f.Tick)
	}
	return fmt.Errorf("%s: its forwarder isn't forwarding", vm)
}

// pid returns the VM's forwarder's pid while it runs.
func (f Forwards) pid(vm string) (int, bool) {
	b, err := os.ReadFile(f.file(vm, ".ports.pid"))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || !ProcessRuns(pid) {
		return 0, false
	}
	args, err := f.Args(pid)
	if err != nil || len(args) < 2 || args[1] != f.Cmd {
		return 0, false
	}
	if i := slices.Index(args, "-vm"); i < 0 || i+1 >= len(args) || args[i+1] != vm {
		return 0, false
	}
	return pid, true
}

// Running reports whether the VM's forwarder runs.
func (f Forwards) Running(vm string) bool {
	_, ok := f.pid(vm)
	return ok
}

// Lock holds the VM's forwarder's lock until unlock is called: whatever
// stops its forwarder, asks which ports are free and starts the next one
// holds it throughout, so two at once (a start and devvm ports, or two
// devvm ports) never leave a forwarder whose pid file names another, which
// nothing would stop. Another devvm holding it is waited for, until ctx
// ends.
func (f Forwards) Lock(ctx context.Context, vm string) (unlock func(), err error) {
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return nil, err
	}
	return platform.LockFile(ctx, f.file(vm, ".ports.lock"))
}

// Stop stops the VM's forwarder, if it runs, waiting up to two seconds for
// it to end, then killing it and waiting up to two more: a start that
// follows finds its ports free.
func (f Forwards) Stop(vm string) {
	pid, ok := f.pid(vm)
	if !ok {
		return
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		_ = p.Signal(sig)
		for range 20 {
			if !ProcessRuns(pid) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// Remove stops the VM's forwarder and deletes its files.
func (f Forwards) Remove(vm string) error {
	f.Stop(vm)
	for _, ext := range []string{".ports.pid", ".ports.known_hosts", ".ports.log", ".ports.lock"} {
		if err := os.Remove(f.file(vm, ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ProcessRuns reports whether a process runs with that pid.
func ProcessRuns(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
