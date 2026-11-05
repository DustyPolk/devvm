// Command devvm creates one isolated VM per project folder.
package main

import (
	"context"
	"os"

	"github.com/w3cj/devvm/internal/cli"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/ui"
)

// main runs devvm through ui.Main, which makes the run's UI, runs the
// cleanup stack and handles Ctrl+C, and exits with its code.
// First, the files it was given past the standard three are kept from the
// processes it starts.
func main() {
	platform.CloseInheritedOnExec()
	os.Exit(ui.Main(func(ctx context.Context, u *ui.UI) int {
		return cli.Execute(ctx, u, os.Args[1:])
	}))
}
