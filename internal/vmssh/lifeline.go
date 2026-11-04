package vmssh

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/w3cj/devvm/internal/daemon"
)

// Main runs a VM's forwarder, as devvm _ports (Cmd) with its arguments
// (Forwards.Start's): ports.sh is script, reserved the ports devvm
// records for VMs (but the VM's own listed ones), and its lines go to
// log. It runs while the VM's lifeline holds (daemon.Lifeline), which
// is the backend's: the pid file of the host process that runs the VM.
func Main(ctx context.Context, args []string, script []byte, reserved func(vm string) (map[int]bool, error), log io.Writer) error {
	fs := flag.NewFlagSet(Cmd, flag.ContinueOnError)
	fs.SetOutput(log)
	var fl Flags
	fl.Register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fl.lifeline == "" {
		return errors.New("-lifeline is needed")
	}
	f, err := fl.Forwarder(script, reserved, log)
	if err != nil {
		return err
	}
	// The VM runs as it starts the forwarder: no wait.
	life, ok := daemon.FollowLifeline(fl.lifeline, 0, f.Tick)
	if !ok {
		return errors.New(fl.VM + ": no lifeline at " + fl.lifeline)
	}
	f.Alive = func(context.Context) bool { return life.Holds() }
	f.Every = f.Tick
	return f.Run(ctx)
}
