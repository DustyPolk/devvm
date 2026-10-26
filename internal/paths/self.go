// Package paths names every folder and file devvm uses: its binary, and
// the folders it keeps state in.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Self returns the running binary's path, after following links.
func Self() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("finding devvm's binary: %w", err)
	}
	return resolve(exe)
}

func resolve(exe string) (string, error) {
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("following links from %s: %w", exe, err)
	}
	return resolved, nil
}
