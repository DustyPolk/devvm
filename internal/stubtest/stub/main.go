// Command stub stands in for the host commands devvm runs, in tests: one
// binary that each command's name links to, which acts as the command
// it's run as (its argv[0]). internal/stubtest builds it once and puts
// its links first on PATH, so tests never run the real tools.
//
// Every stub logs its call to $STUB_LOG, one line of its words joined by
// spaces, written in one write from this process: lines from stubs
// running at once never mix, and the words are never another process's
// arguments (endpoint security tools kill a process whose arguments hold
// an ssh option next to a proxy URL with a password, as a start's
// limactl edit does). What the stubs keep between calls is
// internal/stubtest/stubstate's, in $STUB_DIR; the files a test makes in
// $STUB_DIR are its switches (a stub's header comment lists its own),
// and the files the stubs write there are what a test checks.
package main

import (
	"context"
	"os"
	"path/filepath"
)

func main() {
	c := &call{
		ctx:    context.Background(), //nolint:forbidigo // a stub's run is the process's
		name:   filepath.Base(os.Args[0]),
		args:   os.Args[1:],
		lookup: os.LookupEnv,
		stdin:  os.Stdin,
		stdout: os.Stdout, //nolint:forbidigo // a stub's output is the command's
		stderr: os.Stderr, //nolint:forbidigo // a stub's output is the command's
		ppid:   os.Getppid(),
	}
	os.Exit(c.run()) //nolint:forbidigo // a stub exits as the command it stands for
}
