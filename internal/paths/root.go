// Package paths names every folder and file devvm uses: its own files in
// assets/ next to the binary, and the folders it keeps state in.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// Root returns the folder devvm's files are in: RootOf the running binary,
// after following links.
func Root() (string, error) {
	self, err := Self()
	if err != nil {
		return "", err
	}
	return RootOf(self), nil
}

// RootOf returns the folder the files of devvm's binary at self are in:
// assets/ beside it.
func RootOf(self string) string { return filepath.Join(filepath.Dir(self), "assets") }

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
