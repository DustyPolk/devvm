// Package registry registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package registry

import (
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/paths"
)

// unreviewed is "allowed" only in the builds tests make (tests/e2e.sh, the
// CLI tests), set with -ldflags -X: only there does DEVVM_UNREVIEWED let
// unreviewed emulations through. Releases never set it.
var unreviewed = ""

// UI is what every backend needs of devvm's UI.
type UI interface {
	lima.UI
	smolvm.UI
}

// New returns the registry of devvm's backends, on devvm's folders, its
// own files (assets) and u: lima, the default (backend.Default), and
// smolvm (experimental). In a test build, the unreviewed emulations
// DEVVM_UNREVIEWED names are let through, with a warning each time: for
// their end-to-end checks.
func New(d paths.Dirs, assets fs.FS, u UI) (*backend.Registry, error) {
	return newFor(d, assets, u, unreviewed, os.Getenv("DEVVM_UNREVIEWED"))
}

// newFor is New for a build whose unreviewed is build, with
// DEVVM_UNREVIEWED's value env.
func newFor(d paths.Dirs, assets fs.FS, u UI, build, env string) (*backend.Registry, error) {
	r, err := backend.NewRegistry(lima.New(d, assets, u), smolvm.New(d, assets, u))
	if err != nil {
		return nil, err
	}
	if build == "allowed" {
		r.Unreviewed = backend.ParseUnreviewed(env)
	}
	r.Warn = u.Warn
	return r, nil
}

// Experimental reports whether the backend by that name is experimental:
// smolvm.
func Experimental(name string) bool { return name == smolvm.Name }
