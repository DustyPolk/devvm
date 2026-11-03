// Package registry registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package registry

import (
	"io/fs"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/backend/lima"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/platform"
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
	// KVM (DEVVM_KVM) stands in for Linux's /dev/kvm.
	KVM string
	// UbuntuURL (DEVVM_UBUNTU_URL) is where Ubuntu's dated releases are
	// listed, in place of ubuntu.DefaultURL.
	UbuntuURL string
}

// UI is what every backend needs of devvm's UI.
type UI interface {
	lima.UI
}

// New returns the registry of devvm's backends, on devvm's folders, its
// own files (assets) and u: lima, the default (backend.Default), with a
// test build's overrides o in place of its own. The registry's Host is
// o's (platform.NewHost).
func New(d paths.Dirs, assets fs.FS, u UI, o Overrides) (*backend.Registry, error) {
	host := platform.NewHost(o.Host)
	l := lima.New(d, assets, u, host, o.UbuntuURL)
	if o.KVM != "" {
		l.KVM = o.KVM
	}
	return build(u, host, o.Unreviewed, l)
}

// build returns the registry of bs, on host, warning through u, with the
// unreviewed emulations unreviewed lets through (DEVVM_UNREVIEWED's
// form); a backend whose name is bad or taken fails it.
func build(u UI, host platform.Host, unreviewed string, bs ...backend.Backend) (*backend.Registry, error) {
	r, err := backend.NewRegistry(bs...)
	if err != nil {
		return nil, err
	}
	r.Host = host
	r.Unreviewed = backend.ParseUnreviewed(unreviewed)
	r.Warn = u.Warn
	return r, nil
}
