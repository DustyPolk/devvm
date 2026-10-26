package smolvm

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend/vmssh"
)

// Forwarding every port (ports-all) is emulated, and reviewed: smolvm
// forwards only the ports a machine was made or updated with, at most 64,
// and only to servers listening on all of the guest's addresses. So a VM
// that forwards every port gets none of smolvm's, and devvm's forwarder
// instead (vmssh.Forwarder, devvm _smolvm-ports, started by Start once its
// sshd runs): it reaches the VM's sshd on its forward on the host's
// loopback, and ends when the machine's process does (its lifeline,
// agent.pid).

// forwarderCmd is the hidden command that runs a VM's forwarder.
const forwarderCmd = "_smolvm-ports"

// readLifeline returns what a lifeline holds, "" when it's gone.
func readLifeline(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // smolvm's own file for the machine
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// lifelineHolds reports whether the lifeline still holds life and its
// process (the first line) runs.
func lifelineHolds(path, life string) bool {
	cur, err := readLifeline(path)
	if err != nil || cur != life {
		return false
	}
	first, _, _ := strings.Cut(cur, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	return err == nil && pid > 0 && vmssh.ProcessRuns(pid)
}

// ForwardMain runs a VM's forwarder (devvm _smolvm-ports) with its
// arguments: ports.sh is script, reserved the ports devvm records for VMs
// (but the VM's own listed ones), and its lines go to log. Besides the
// forwarder's own flags, it takes the machine's lifeline and the host port
// its sshd is forwarded from.
func ForwardMain(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	fs := flag.NewFlagSet(forwarderCmd, flag.ContinueOnError)
	fs.SetOutput(log)
	var fl vmssh.Flags
	var lifeline string
	fl.Register(fs)
	fs.StringVar(&lifeline, "lifeline", "", "the machine's agent.pid: exit when it changes or its process is gone")
	fs.IntVar(&fl.Port, "port", 0, "the host loopback port the VM's sshd is forwarded from")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if lifeline == "" {
		return errors.New("-lifeline is needed")
	}
	f, err := fl.Forwarder(script, reserved, log)
	if err != nil {
		return err
	}
	life, err := readLifeline(lifeline)
	if err != nil || life == "" {
		return errors.New(fl.VM + ": no lifeline at " + lifeline)
	}
	f.Alive = func(context.Context) bool { return lifelineHolds(lifeline, life) }
	f.Every = f.Tick
	return f.Run(ctx)
}
