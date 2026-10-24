// Command devvm creates one isolated VM per project folder.
package main

import (
	"os"

	"github.com/w3cj/devvm/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:]))
}
