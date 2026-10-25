// Command devvm creates one isolated VM per project folder.
package main

import (
	"context"
	"os"

	"github.com/w3cj/devvm/internal/cli"
	"github.com/w3cj/devvm/internal/ui"
)

// main runs devvm through ui.Main, which makes the run's UI, recovers its
// Die, runs the cleanup stack and handles Ctrl+C, and exits with its code.
func main() {
	os.Exit(ui.Main(func(ctx context.Context, u *ui.UI) int {
		return cli.Execute(ctx, u, os.Args[1:])
	}))
}
