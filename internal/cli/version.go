package cli

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/ui"
)

// version is devvm's version. Release builds set it:
// -ldflags "-X github.com/w3cj/devvm/internal/cli.version=0.1.0".
var version string

// devvmVersion is the release's version, else the one Go recorded for the
// build (a checkout's pseudo-version), without the v; (devel) when there's
// neither.
func devvmVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "(devel)"
}

// addVersion gives root --version, which prints the version alone. The
// flag is declared here so Cobra doesn't add -v.
func addVersion(root *cobra.Command) {
	root.Version = devvmVersion()
	root.SetVersionTemplate("{{.Version}}\n")
	root.Flags().Bool("version", false, "print devvm's version")
}

// newVersionCmd is devvm version, the same as devvm --version. Hidden:
// --version is the one the help shows.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "version",
		Short:             "Print devvm's version",
		Hidden:            true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := ui.Stdout().Write([]byte(cmd.Root().Version + "\n"))
			return err
		},
	}
}
