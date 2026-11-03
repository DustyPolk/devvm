package vmssh

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/w3cj/devvm/internal/daemon"
)

// LifelineMain runs the forwarder of a VM whose sshd is forwarded to a
// host loopback port (Lima's), as the hidden command name with
// its arguments: ports.sh is script, reserved the ports devvm records for
// VMs (but the VM's own listed ones), and its lines go to log. Besides the
// forwarder's own flags, it takes the VM's lifeline (daemon.Lifeline),
// which it runs while it holds, and that port.
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
	// The VM runs as it starts the forwarder: no wait.
	life, ok := daemon.FollowLifeline(lifeline, 0, f.Tick)
	if !ok {
		return errors.New(fl.VM + ": no lifeline at " + lifeline)
	}
	f.Alive = func(context.Context) bool { return life.Holds() }
	f.Every = f.Tick
	return f.Run(ctx)
}
