// Package registry registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package registry

import (
	"io/fs"
	"os"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/docker"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
)

// unreviewed is "allowed" only in the builds tests make (tests/e2e.sh, the
// CLI tests), set with -ldflags -X: only there does DEVVM_UNREVIEWED let
// unreviewed emulations through, and do the host's overrides
// (platform.NewHost) count. Releases never set it.
var unreviewed = ""

// linuxURL and linuxSHA256 are set in a release's macOS builds, with
// -ldflags -X: its own version's Linux archive, and the SHA-256 of the
// devvm in it, which the Docker backend fetches and checks before use
// (scripts/release.sh). Other builds have none, and fetch nothing.
var (
	linuxURL    = ""
	linuxSHA256 = ""
)

// UI is what every backend needs of devvm's UI.
type UI interface {
	lima.UI
	smolvm.UI
	docker.UI
}

// New returns the registry of devvm's backends, on devvm's folders, its
// own files (assets) and u: lima, the default (backend.Default), smolvm
// and docker (both experimental). In a test build, the unreviewed emulations
// DEVVM_UNREVIEWED names are let through, with a warning each time: for
// their end-to-end checks; DEVVM_DOCKER_KIND sets the kind of Docker's
// engine (linux, desktop, other), and DEVVM_DOCKER_PROC_SYS where its host's
// /proc/sys is, for the CLI tests' stand-in engine; and
// DEVVM_LINUX_BUILD_URL with DEVVM_LINUX_BUILD_SHA256 stand in for a
// release's Linux build, fetched as on macOS whatever the host. The
// registry's Host is Host's.
func New(d paths.Dirs, assets fs.FS, u UI) (*backend.Registry, error) {
	return newFor(d, assets, u, unreviewed, os.Getenv)
}

// Host returns the host devvm runs on, as this build reads it: a test
// build's counts the environment's overrides (platform.TestCPUs and the
// rest), a release's never does.
func Host() platform.Host { return hostFor(unreviewed, os.Getenv) }

// hostFor is Host for a build whose unreviewed is build, reading the
// environment with getenv.
func hostFor(build string, getenv func(string) string) platform.Host {
	return platform.NewHost(build == "allowed", getenv)
}

// newFor is New for a build whose unreviewed is build, reading the
// environment with getenv.
func newFor(d paths.Dirs, assets fs.FS, u UI, build string, getenv func(string) string) (*backend.Registry, error) {
	host := hostFor(build, getenv)
	dk := docker.New(d, assets, u)
	dk.LinuxURL, dk.LinuxSHA256 = linuxURL, linuxSHA256
	if build == "allowed" && getenv("DEVVM_LINUX_BUILD_URL") != "" {
		dk.LinuxURL, dk.LinuxSHA256 = getenv("DEVVM_LINUX_BUILD_URL"), getenv("DEVVM_LINUX_BUILD_SHA256")
		dk.LinuxOS = "darwin"
	}
	r, err := backend.NewRegistry(lima.New(d, assets, u, host), smolvm.New(d, assets, u, host), dk)
	if err != nil {
		return nil, err
	}
	r.Host = host
	if build == "allowed" {
		r.Unreviewed = backend.ParseUnreviewed(getenv("DEVVM_UNREVIEWED"))
		dk.Kind = getenv("DEVVM_DOCKER_KIND")
		if p := getenv("DEVVM_DOCKER_PROC_SYS"); p != "" {
			dk.ProcSys = p
		}
	}
	r.Warn = u.Warn
	return r, nil
}

// Names are every backend's name, sorted, as New registers them: what
// --backend takes.
func Names() []string { return []string{docker.Name, lima.Name, smolvm.Name} }

// Experimental reports whether the backend by that name is experimental:
// smolvm and docker.
func Experimental(name string) bool { return name == smolvm.Name || name == docker.Name }
