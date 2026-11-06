//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// Proc is a host command a section runs in the background (Start): a
// server, a company proxy. It runs in a process group of its own until
// Stop, or the run's end.
type Proc struct {
	cmd  *exec.Cmd
	out  *lockedBuffer
	done chan struct{}
}

// Start runs c in the background, in the sandbox's environment, past the
// check or steps that start it: the run's context ends it, not the
// window's. Its output and errors are Out, never in the check's log, as
// a server's would bury the check's own. Stop ends it, and so does the
// run's end.
func (s *S) Start(c Cmd) (*Proc, error) {
	cmd := exec.CommandContext(s.r.ctx, c.Args[0], c.Args[1:]...)
	cmd.Env = append(slices.Clone(s.sb.env), c.Env...)
	cmd.Dir = c.Dir
	if cmd.Dir == "" {
		cmd.Dir = s.sb.work
	}
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	p := &Proc{cmd: cmd, out: &lockedBuffer{}, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = p.out, p.out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = killAfter
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", strings.Join(c.Args, " "), err)
	}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	s.Defer(func(context.Context) { p.Stop() })
	return p, nil
}

// Running is whether it hasn't ended.
func (p *Proc) Running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Out is what it printed so far, output and errors as they came.
func (p *Proc) Out() string { return p.out.String() }

// Stop ends its process group, TERM first and KILL killAfter later, and
// waits for it to end. Once it has, Stop does nothing.
func (p *Proc) Stop() {
	if !p.Running() {
		return
	}
	pid := p.cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(killAfter):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		<-p.done
	}
}

// listening waits up to timeout for addr to take TCP connections, while
// p runs: a server Start started is ready.
func (p *Proc) listening(ctx context.Context, addr string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	d := &net.Dialer{Timeout: time.Second}
	for {
		c, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			return c.Close()
		}
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case !p.Running():
			return fmt.Errorf("it ended before it listened on %s: %s", addr, strings.TrimSpace(p.Out()))
		case time.Now().After(end):
			return fmt.Errorf("nothing listened on %s within %s", addr, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// ProcWatch samples every host process's command line, every 0.1 s,
// from WatchProcs until Stop: for a
// check that a secret was in no process's arguments while devvm ran.
// Its samples go to the sandbox's ps.log, never the check's log, which a
// failure prints.
type ProcWatch struct {
	file    string
	stop    chan struct{}
	done    chan struct{}
	samples int
}

// WatchProcs starts sampling. A check that reads it is ProcsFlaky.
func WatchProcs(s *S) (*ProcWatch, error) {
	f := filepath.Join(s.sb.work, "ps.log")
	madeFile(s.sb, f)
	fh, err := os.OpenFile(f, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	w := &ProcWatch{file: f, stop: make(chan struct{}), done: make(chan struct{})}
	ctx := s.r.ctx
	go func() {
		defer close(w.done)
		defer fh.Close()
		for {
			// One ps at a time, each waited for: stopping never leaves one
			// running, which could record the next check's command lines.
			if b, err := psAll(ctx); err == nil {
				if _, err := fh.Write(b); err == nil {
					w.samples++
				}
			}
			select {
			case <-w.stop:
				return
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	s.Defer(func(context.Context) { w.Stop() })
	return w, nil
}

// Stop ends the sampling once its current sample is in, and waits for
// it. Once it has, Stop does nothing.
func (w *ProcWatch) Stop() {
	select {
	case <-w.done:
		return
	default:
	}
	close(w.stop)
	<-w.done
}

// Saw stops the watch, then is whether any sample had secret in a
// command line. It fails when there was no sample, as nothing was
// watched then.
func (w *ProcWatch) Saw(secret string) (bool, error) {
	w.Stop()
	if w.samples == 0 {
		return false, errors.New("the process watch took no sample")
	}
	b, err := os.ReadFile(w.file)
	if err != nil {
		return false, err
	}
	return bytes.Contains(b, []byte(secret)), nil
}

// ProcsFlaky is the Flaky of a check that reads a ProcWatch or ProcsHave.
func ProcsFlaky() CheckOpt {
	return Flaky("the process listings, sampled while devvm ran and taken again here, can catch a short-lived process mid-start")
}

// ProcsHave is whether a host process has secret in its command line
// now: one sample, never in the check's log.
func ProcsHave(s *S, secret string) (bool, error) {
	b, err := psAll(s.r.window().ctx)
	if err != nil {
		return false, err
	}
	return bytes.Contains(b, []byte(secret)), nil
}

// psAll is every process's command line, one a line. ps's own has no
// secret: the caller looks for it in the output.
func psAll(ctx context.Context) ([]byte, error) {
	b, err := exec.CommandContext(ctx, "ps", "-axww", "-o", "command=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return b, nil
}
