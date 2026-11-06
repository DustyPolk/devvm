// Package registry registers every backend devvm has. It's apart from
// package backend, which the backends themselves import.
package registry

import (
	"io/fs"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/backend/lima"
	"github.com/devvmhq/devvm/internal/msg"
	"github.com/devvmhq/devvm/internal/paths"
	"github.com/devvmhq/devvm/internal/platform"
	"github.com/devvmhq/devvm/internal/settings"
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
	// UbuntuRelease (DEVVM_UBUNTU_RELEASE) is the dated release of
	// Ubuntu's cloud image that base is pinned to, in place of the newest:
	// e2e's, so its runs keep their images.
	UbuntuRelease string
}

// New returns the registry of devvm's backends, on devvm's folders, its
// own files (assets) and r: lima, the default (backend.Default), with a
// test build's overrides o in place of its own, their own requests on
// this machine through the company proxy cp. The registry's Host is o's
// (platform.NewHost).
func New(d paths.Dirs, assets fs.FS, r msg.Reporter, o Overrides, cp *settings.CompanyProxy) (*backend.Registry, error) {
	host := platform.NewHost(o.Host)
	l := lima.New(d, assets, r, host, o.UbuntuURL, cp)
	if o.KVM != "" {
		l.KVM = o.KVM
	}
	l.UbuntuRelease = o.UbuntuRelease
	return build(r, host, o.Unreviewed, l)
}

// build returns the registry of bs, on host, warning through rep, with
// the unreviewed emulations unreviewed lets through (DEVVM_UNREVIEWED's
// form); a backend whose name is bad or taken fails it.
func build(rep msg.Reporter, host platform.Host, unreviewed string, bs ...backend.Backend) (*backend.Registry, error) {
	r, err := backend.NewRegistry(bs...)
	if err != nil {
		return nil, err
	}
	r.Host = host
	r.Unreviewed = backend.ParseUnreviewed(unreviewed)
	r.Reporter = rep
	return r, nil
}
