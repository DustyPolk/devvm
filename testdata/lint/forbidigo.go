// Package lint breaks each of .golangci.yml's forbidigo rules once, on a
// line ending in "// want forbidigo". mise run lint:code checks that
// forbidigo reports those lines and no others.
package lint

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"syscall"

	"github.com/w3cj/devvm/internal/testbuild"
)

func exits() { os.Exit(1) } // want forbidigo

func prints() { fmt.Println("x") } // want forbidigo

func writes() { _, _ = os.Stdout.WriteString("x") } // want forbidigo

func contexts() context.Context { return context.Background() } // want forbidigo

func cpus() int { return runtime.NumCPU() } // want forbidigo

func testBuild() bool { return testbuild.On() } // want forbidigo

func signals(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) } // want forbidigo

func finds(pid int) (*os.Process, error) { return os.FindProcess(pid) } // want forbidigo

func client() any { return &http.Client{} } // want forbidigo
