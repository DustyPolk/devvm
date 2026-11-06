// Command docgen writes the CLI reference, docs/content/docs/cli/, from
// devvm's commands: one Hugo page per command users can run, and the
// section's _index.md. Run it with mise run cli-docs.
package main

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/devvmhq/devvm/internal/cli"
	"github.com/devvmhq/devvm/internal/ui"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: docgen DIR")
	}
	if err := write(newRoot(), os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

// newRoot is devvm's command, for its pages only: none of it runs, so its
// UI has no input and prints nowhere.
func newRoot() *cobra.Command {
	return cli.NewRoot(ui.New(strings.NewReader(""), io.Discard, io.Discard, func(string) string { return "" }))
}
