// Package registry registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package registry

import (
	"io/fs"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/docker"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/backend/smolvm"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
	"github.com/w3cj/devvm/internal/settings"
)

// Overrides are a test build's stand-ins for the backends, each the value
// of the environment variable it names, as internal/cli reads them (only
// in a test build: a release has none). "" leaves the backends' own.
type Overrides struct {
	// Host stands in for this machine's size (platform.Overrides).
	Host platform.Overrides
	// Unreviewed (DEVVM_UNREVIEWED) names the unreviewed emulations let
	// through, with a warning each time: for their end-to-end checks.
	Unreviewed string
	// DockerKind (DEVVM_DOCKER_KIND) is the kind of Docker's engine
	// (linux, desktop, other), and DockerProcSys (DEVVM_DOCKER_PROC_SYS)
	// where its host's /proc/sys is, for the CLI tests' stand-in engine.
	DockerKind, DockerProcSys string
	// LinuxBuildURL and LinuxBuildSHA256 (DEVVM_LINUX_BUILD_URL,
	// DEVVM_LINUX_BUILD_SHA256) stand in for a release's Linux build,
	// fetched as on macOS whatever the host.
	LinuxBuildURL, LinuxBuildSHA256 string
	// KVM (DEVVM_KVM) stands in for Linux's /dev/kvm.
	KVM string
	// UbuntuURL (DEVVM_UBUNTU_URL) is where Ubuntu's dated releases are
	// listed, in place of ubuntu.DefaultURL.
	UbuntuURL string
}

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
// and docker (both experimental), with a test build's overrides o in place
// of their own. The registry's Host is o's (platform.NewHost).
func New(d paths.Dirs, assets fs.FS, u UI, o Overrides) (*backend.Registry, error) {
	host := platform.NewHost(o.Host)
	dk := docker.New(d, assets, u)
	dk.LinuxURL, dk.LinuxSHA256 = linuxURL, linuxSHA256
	if o.LinuxBuildURL != "" {
		dk.LinuxURL, dk.LinuxSHA256 = o.LinuxBuildURL, o.LinuxBuildSHA256
		dk.LinuxOS = "darwin"
	}
	dk.Kind = o.DockerKind
	if o.DockerProcSys != "" {
		dk.ProcSys = o.DockerProcSys
	}
	l, s := lima.New(d, assets, u, host, o.UbuntuURL), smolvm.New(d, assets, u, host)
	if o.KVM != "" {
		l.KVM, s.KVM = o.KVM, o.KVM
	}
	r, err := backend.NewRegistry(l, s, dk)
	if err != nil {
		return nil, err
	}
	r.Host = host
	r.Unreviewed = backend.ParseUnreviewed(o.Unreviewed)
	r.Warn = u.Warn
	r.DefaultBackend = func() (string, string, error) { return defaultBackend(d) }
	return r, nil
}

// defaultBackend is the backend new VMs get without --backend, as d's
// config.toml or settings.json names it, with that file; backend.Default
// and "" when neither does.
func defaultBackend(d paths.Dirs) (name, file string, err error) {
	def, err := settings.Read(d)
	return def.Backend.Value, def.Backend.Source.File(d), err
}

// Names are every backend's name, sorted, as New registers them: what
// --backend takes.
func Names() []string { return []string{docker.Name, lima.Name, smolvm.Name} }

// Experimental reports whether the backend by that name is experimental:
// smolvm and docker.
func Experimental(name string) bool { return name == smolvm.Name || name == docker.Name }
