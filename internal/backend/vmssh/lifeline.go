package vmssh

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"strconv"
	"strings"
)

// A lifeline is the file of the process that runs a VM (smolvm's
// agent.pid, Lima's hostagent's ha.pid), its pid on the first line, which
// each start writes anew: a forwarder runs while it holds what it held
// when the forwarder started.

// ReadLifeline returns what a lifeline holds, "" when it's gone.
func ReadLifeline(path string) (string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the backend's own file for the VM
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(b)), err
}

// LifelineHolds reports whether the lifeline still holds life and its
// process (the first line) runs.
func LifelineHolds(path, life string) bool {
	cur, err := ReadLifeline(path)
	if err != nil || cur != life {
		return false
	}
	first, _, _ := strings.Cut(cur, "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(first))
	return err == nil && pid > 0 && ProcessRuns(pid)
}

// LifelineMain runs the forwarder of a VM whose sshd is forwarded to a
// host loopback port (smolvm's, Lima's), as the hidden command name with
// its arguments: ports.sh is script, reserved the ports devvm records for
// VMs (but the VM's own listed ones), and its lines go to log. Besides the
// forwarder's own flags, it takes the VM's lifeline, which it runs while
// it holds, and that port.
func LifelineMain(ctx context.Context, name string, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(log)
	var fl Flags
	var lifeline string
	fl.Register(fs)
	fs.StringVar(&lifeline, "lifeline", "", "the VM's process's pid file: exit when it changes or its process is gone")
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
	life, err := ReadLifeline(lifeline)
	if err != nil || life == "" {
		return errors.New(fl.VM + ": no lifeline at " + lifeline)
	}
	f.Alive = func(context.Context) bool { return LifelineHolds(lifeline, life) }
	f.Every = f.Tick
	return f.Run(ctx)
}
