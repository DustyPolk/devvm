package vm

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/image"
)

// nixMarkers are a Nix project's own files, which call for the nix layer.
var nixMarkers = []string{"flake.nix", "shell.nix", "devenv.nix"}

// marker is a part and the files in a folder's top level that call for it.
type marker struct {
	part  string
	files []string
}

// markers are the files that call for each toolchain (image.SharedTools'
// Detect), then for the nix layer, in the order detectFiles lists them:
// the same mix always gives the same image.
func markers() []marker {
	var ms []marker
	for _, d := range image.SharedTools {
		if len(d.Detect) > 0 {
			ms = append(ms, marker{d.Name, d.Detect})
		}
	}
	return append(ms, marker{image.NixLayer, nixMarkers})
}

// nixShellMax is how much of a default.nix nixShell reads.
const nixShellMax = 64 << 10

// nixShell reports whether dir's default.nix makes a shell (mkShell, or
// mkShellNoCC), as one for nix-shell does; most build a package instead.
func nixShell(dir string) bool {
	p := filepath.Join(dir, "default.nix")
	if fi, err := os.Lstat(p); err != nil || !fi.Mode().IsRegular() {
		return false
	}
	f, err := os.Open(p) //nolint:gosec // a regular file in the project folder, read
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(io.LimitReader(f, nixShellMax))
	return strings.Contains(string(data), "mkShell")
}

// detected is a toolchain, or the nix layer, that a folder calls for, and
// the first of its marker files that calls for it.
type detected struct{ Name, File string }

// detectFiles returns the toolchains and the nix layer whose marker files
// are in dir's top level, in a fixed order, each with the file that calls
// for it: nix also for a default.nix that makes a shell. A new VM's
// default parts (defaults) start from them.
func detectFiles(dir string) []detected {
	var found []detected
	for _, m := range markers() {
		file := ""
		for _, f := range m.files {
			if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
				file = f
				break
			}
		}
		if file == "" && m.part == image.NixLayer && nixShell(dir) {
			file = "default.nix"
		}
		if file != "" {
			found = append(found, detected{m.part, file})
		}
	}
	return found
}
