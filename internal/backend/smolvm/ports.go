package smolvm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_smolvm-ports"

// Forwards starts and stops the VMs' port forwarders (forward.go): devvm
// _smolvm-ports, detached, one per VM that forwards every port.
type Forwards struct {
	Dirs paths.Dirs
	// Self is devvm's binary, which runs as the forwarder.
	Self string
	// Detach starts a process that outlives devvm (platform.StartDetached).
	Detach func(path string, args []string, out *os.File) (int, error)
	// Args returns a running process's arguments (platform.ProcessArgs).
	Args func(pid int) ([]string, error)
	// Tick is how often a start looks for the forwarder to be ready, 20
	// times at most.
	Tick time.Duration
}

// forwardSpec is what a VM's forwarder needs: the machine's lifeline, the
// VM's sshd's host port and user, the VM's key, its host key (TYPE
// BASE64), and the host ports it never forwards.
type forwardSpec struct {
	lifeline, user, key, hostKey string
	sshPort                      int
	reserve                      []int
}

// file is one of the forwarder's files, beside the VM's record.
func (f Forwards) file(vm, ext string) string {
	return filepath.Join(filepath.Dir(f.Dirs.SmolvmVM(vm)), vm+ext)
}

// Log is the VM's forwarder's log.
func (f Forwards) Log(vm string) string { return f.file(vm, ".ports.log") }

// control is the master's control socket: short, as a socket's path is at
// most 104 characters and ssh adds 17 while it makes it.
func (f Forwards) control(vm string) string {
	sum := sha256.Sum256([]byte(vm))
	return filepath.Join(f.Dirs.Run(), "smolvm-"+hex.EncodeToString(sum[:])[:16]+".ctl")
}

// logMax is the most a forwarder's log keeps from earlier starts: past
// it, a start empties it.
const logMax = 1 << 20

// controlMax is the longest control socket path ssh can make.
const controlMax = 104 - 1 - 17

// Start starts the VM's forwarder, after any it had, and waits for it to
// say it forwards. One that doesn't is left to keep trying, and its log
// named in the error.
func (f Forwards) Start(vm string, spec forwardSpec) error {
	f.Stop(vm)
	ctl := f.control(vm)
	if len(ctl) > controlMax {
		return fmt.Errorf("%s: its path is too long for a socket", ctl)
	}
	if err := os.MkdirAll(filepath.Dir(ctl), 0o700); err != nil {
		return err
	}
	kh := f.file(vm, ".ports.known_hosts")
	if err := state.WriteFile(kh, []byte(vm+" "+spec.hostKey+"\n")); err != nil {
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
	for _, p := range spec.reserve {
		if p > 0 {
			reserve = append(reserve, strconv.Itoa(p))
		}
	}
	pid, err := f.Detach(f.Self, []string{
		forwarderCmd, "-vm", vm, "-lifeline", spec.lifeline, "-port", strconv.Itoa(spec.sshPort), "-user", spec.user,
		"-key", spec.key, "-known-hosts", kh, "-control", ctl, "-reserve", strings.Join(reserve, ","),
	}, out)
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
	if err != nil || pid <= 0 || !alive(pid) {
		return 0, false
	}
	args, err := f.Args(pid)
	if err != nil || len(args) < 2 || args[1] != forwarderCmd {
		return 0, false
	}
	if i := slices.Index(args, "-vm"); i < 0 || i+1 >= len(args) || args[i+1] != vm {
		return 0, false
	}
	return pid, true
}

// Stop stops the VM's forwarder, if it runs, waiting up to two seconds for
// it to end.
func (f Forwards) Stop(vm string) {
	pid, ok := f.pid(vm)
	if !ok {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	for range 20 {
		if !alive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Remove stops the VM's forwarder and deletes its files.
func (f Forwards) Remove(vm string) error {
	f.Stop(vm)
	for _, ext := range []string{".ports.pid", ".ports.known_hosts", ".ports.log"} {
		if err := os.Remove(f.file(vm, ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// hostKeyLine is an ed25519 public key line, as the VM's sshd has it.
var hostKeyLine = regexp.MustCompile(`^ssh-ed25519 [A-Za-z0-9+/]+=*(\s|$)`)

// hostKey returns the key type and base64 of the VM's sshd's host key, as
// cat of its .pub file gives them.
func hostKey(out []byte) (string, bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if !hostKeyLine.MatchString(line) {
		return "", false
	}
	f := strings.Fields(line)
	return f[0] + " " + f[1], true
}

// deciderPort is the port of a decider's address (127.0.0.1:PORT), 0 for
// none.
func deciderPort(addr string) int {
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return 0
	}
	return int(ap.Port())
}
