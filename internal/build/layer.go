package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/w3cj/devvm/internal/image"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// layerScript is the script a new layer starts with; NAME is its name.
const layerScript = `#!/bin/bash
# NAME: a devvm layer. devvm runs it once, with bash as the VM's user (who has sudo), in an image
# on top of base: devvm build NAME, or devvm node+NAME for a VM. Editing it rebuilds that image,
# and the ones built on it, on next use.
# - Put PATH changes in ~/.profile.
# - Download with devvm-fetch URL SHA256 OUT: it fails when the checksum doesn't match.
# - With network policy, NAME.policy next to this file lists the hosts VMs with this layer reach.
set -euo pipefail

# sudo apt-get install -y postgresql-client
`

// layerPolicy is the allowlist a new layer starts with.
const layerPolicy = `# Hosts a VM with the NAME layer can reach with network policy, one rule per line: example.com,
# *.example.com (any subdomain), host:port, an address or CIDR, !rule (deny).
`

// NewLayer starts one of the user's layers: its script, and its allowlist
// unless that exists. It returns their paths. A name a layer can't have,
// or whose script exists, is refused.
func NewLayer(dirs paths.Dirs, name string) (script, policy string, err error) {
	switch {
	case name == image.Base || image.IsTool(name) || image.IsBuiltin(name):
		return "", "", msg.LayerNameReserved(name)
	case !image.LayerName(name):
		return "", "", msg.LayerNameInvalid(name)
	}
	script = filepath.Join(dirs.Layers(), name+".sh")
	policy = filepath.Join(dirs.Layers(), name+".policy")
	if _, err := os.Lstat(script); err == nil {
		return "", "", msg.LayerExists(script)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	if err := os.MkdirAll(dirs.Layers(), 0o700); err != nil {
		return "", "", err
	}
	if err := createNew(script, strings.ReplaceAll(layerScript, "NAME", name)); errors.Is(err, fs.ErrExist) {
		return "", "", msg.LayerExists(script)
	} else if err != nil {
		return "", "", err
	}
	if err := createNew(policy, strings.ReplaceAll(layerPolicy, "NAME", name)); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", "", err
	}
	return script, policy, nil
}

// createNew writes a file that mustn't exist yet, whole or not at all: a
// temporary file, linked into place.
func createNew(path, data string) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	_, err = f.WriteString(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Link(tmp, path)
	}
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return err
}
