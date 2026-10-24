// Command docgen writes the CLI reference, docs/content/docs/cli/, from
// devvm's commands: one Hugo page per command users can run, and the
// section's _index.md. Run it with mise run cli-docs.
package main

import (
	"log"
	"os"

	"github.com/w3cj/devvm/internal/cli"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: docgen DIR")
	}
	if err := write(cli.NewRoot(), os.Args[1]); err != nil {
		log.Fatal(err)
	}
}
