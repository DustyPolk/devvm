// Package paths finds devvm's own files and the folders it keeps state in.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root returns the folder devvm's files are in: the one holding the
// running binary, after following links.
func Root() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding devvm's binary: %w", err)
	}
	return rootOf(exe)
}

func rootOf(exe string) (string, error) {
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("following links from %s: %w", exe, err)
	}
	return filepath.Dir(resolved), nil
}
