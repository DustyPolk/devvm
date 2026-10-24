package ui

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"syscall"
)

// InterruptedCode is the exit code of a run ended by Ctrl+C or a closed
// terminal (INT, TERM, HUP).
const InterruptedCode = 130

// exitError is what Die panics with, for Main to recover.
type exitError struct{ code int }

func exit(code int) { panic(exitError{code}) }

var (
	interrupted atomic.Bool

	// commands is read-locked while a logged command runs, so an interrupt
	// can wait for the run's commands to end before cleaning up.
	commands sync.RWMutex

	cleanups struct {
		sync.Mutex
		next  int
		funcs []cleanup
	}
)

type cleanup struct {
	id int
	f  func(ctx context.Context)
}

// Interrupted reports whether the run was interrupted (Ctrl+C, TERM, a
// closed terminal). A cleanup that only undoes unfinished work on an
// interrupt checks it.
func Interrupted() bool { return interrupted.Load() }

// AddCleanup adds f to the cleanup stack, which runs, last added first, when
// the run ends: normally, through Die, or interrupted. f gets a context of
// its own, as the run's is canceled by then. Call remove once there's
// nothing left to clean up.
func AddCleanup(f func(ctx context.Context)) (remove func()) {
	cleanups.Lock()
	defer cleanups.Unlock()
	cleanups.next++
	id := cleanups.next
	cleanups.funcs = append(cleanups.funcs, cleanup{id, f})
	return func() {
		cleanups.Lock()
		defer cleanups.Unlock()
		for i, c := range cleanups.funcs {
			if c.id == id {
				cleanups.funcs = append(cleanups.funcs[:i], cleanups.funcs[i+1:]...)
				return
			}
		}
	}
}

// runCleanups runs the cleanup stack, last added first. A cleanup that dies
// or panics doesn't stop the rest.
func runCleanups() {
	for {
		cleanups.Lock()
		n := len(cleanups.funcs)
		if n == 0 {
			cleanups.Unlock()
			return
		}
		c := cleanups.funcs[n-1]
		cleanups.funcs = cleanups.funcs[:n-1]
		cleanups.Unlock()
		func() {
			defer func() { _ = recover() }()
			c.f(context.Background())
		}()
	}
}

type result struct {
	code  int
	panic any // a panic that isn't Die's, with its stack
}

// call runs fn, turning Die's panic into its exit code.
func call(ctx context.Context, fn func(context.Context) int) (r result) {
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(exitError); ok {
				r = result{code: e.code}
				return
			}
			r = result{panic: fmt.Sprintf("%v\n\n%s", p, debug.Stack())}
		}
	}()
	return result{code: fn(ctx)}
}

// Main runs fn, the whole of devvm's run, and returns the exit code: fn's,
// or 1 after Die. Then it runs the cleanup stack.
//
// INT, TERM and HUP cancel fn's context, which stops the command running,
// and then run the cleanups and return InterruptedCode. A second signal
// during the cleanups is ignored, so they aren't stopped halfway. fn's own
// code keeps running until it next starts a command (which fails, its
// context being canceled) or dies, quietly.
func Main(fn func(ctx context.Context) int) int {
	interrupted.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)

	done := make(chan result, 1)
	go func() { done <- call(ctx, fn) }()
	var r result
	select {
	case r = <-done:
	case <-sigs:
		interrupted.Store(true)
		cancel()
		stopStatus()
		commands.Lock() // the commands running have ended
		commands.Unlock()
		write(stderr, "\n")
		r = result{code: InterruptedCode}
	}
	stopStatus()
	runCleanups()
	if r.panic != nil {
		panic(r.panic)
	}
	return r.code
}
