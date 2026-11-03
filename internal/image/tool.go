package image

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/w3cj/devvm/internal/msg"
)

// Toolchains are the shared toolchains a VM can have, in the fixed order
// a VM's list holds them (project.DetectFiles').
var Toolchains = []string{"node", "python", "rust", "go"}

// Agents are the shared coding agents a VM can have, listed after its
// toolchains.
var Agents = []string{"claude", "codex", "opencode"}

// IsTool reports whether a name is a shared tool's (a toolchain or an
// agent) rather than a layer's.
func IsTool(name string) bool {
	return slices.Contains(Toolchains, name) || slices.Contains(Agents, name)
}

// IsAgent reports whether a name is an agent's.
func IsAgent(name string) bool { return slices.Contains(Agents, name) }

// SortTools returns shared tools' names in a VM's order, each once: the
// toolchains in their fixed order, then the agents. Names that aren't
// shared tools are left out.
func SortTools(names []string) []string {
	var out []string
	for _, t := range slices.Concat(Toolchains, Agents) {
		if slices.Contains(names, t) {
			out = append(out, t)
		}
	}
	return out
}

// AgentsOf returns the agents among shared tools' names, in their order.
func AgentsOf(names []string) []string {
	var out []string
	for _, n := range names {
		if IsAgent(n) {
			out = append(out, n)
		}
	}
	return out
}

// GroupTools returns the tools in the lock behind a shared tool's name:
// node is node, pnpm and bun; python is python and uv.
func GroupTools(name string) []string {
	switch name {
	case "node":
		return []string{"node", "pnpm", "bun"}
	case "python":
		return []string{"python", "uv"}
	}
	return []string{name}
}

// Normalize returns an image name's canonical form, so the same layers
// always make one image: the layers bottom-up as given, each once, without
// base, which every image is built on; base alone when there's nothing
// else. An empty name stays empty, for CheckLayers to refuse.
func Normalize(name string) string {
	if name == "" {
		return ""
	}
	return Of(strings.Split(name, "+"))
}

// Of is the image with these layers, bottom-up: base when there are none.
// base and repeats are dropped.
func Of(layers []string) string {
	var parts []string
	for _, p := range layers {
		if p != Base && p != "" && !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return Base
	}
	return strings.Join(parts, "+")
}

// LayersOf is an image's layers, bottom-up: none for base.
func LayersOf(name string) []string {
	if name == Base || name == "" {
		return nil
	}
	return strings.Split(name, "+")
}

// Tool is a shared tool at the version the tools lock gives it.
type Tool struct {
	// Name is the tool's name in the lock: node, claude.
	Name string
	// Version is the lock's version of it.
	Version string
	// LockEntry is the tool's entry in the lock, with its downloads and
	// their checksums, as written there.
	LockEntry []byte
	// Libc is the C library it's built for: gnu, or musl.
	Libc string
	// Format is the image's EROFS format, as its backend names it (the
	// backend's ToolFormat): "" for shared-tools/build.sh's own, compressed.
	Format string
}

var (
	toolName    = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	toolVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]*$`)
)

// ToolName reports whether a shared tool can have that name, as the lock
// and a tool image's file name have it: a lowercase letter, then
// lowercase letters and digits.
func ToolName(s string) bool { return toolName.MatchString(s) }

// ToolVersion reports whether s is shaped like a version as upstream may
// name one: a letter or digit, then letters, digits and ._+-. It goes into
// file names, mounts, the build VM's commands and mise's TOML.
func ToolVersion(s string) bool { return toolVersion.MatchString(s) }

// ToolFile names a tool's image: <tool>-<version>-<key>.erofs. The key is
// 12 hex digits of a SHA-256 over the tool's lock entry, devvm's
// shared-tools/build.sh (in assets, devvm's own files), the [settings] of
// devvm's shared-tools/mise.toml, which change what mise installs, the
// libc and, unless it's build.sh's own, the format.
func ToolFile(assets fs.FS, t Tool) (string, error) {
	if !ToolName(t.Name) {
		return "", fmt.Errorf("invalid tool name %q", t.Name)
	}
	if !toolVersion.MatchString(t.Version) {
		return "", msg.ToolBadVersion(t.Name, strconv.Quote(t.Version))
	}
	if t.Libc != "gnu" && t.Libc != "musl" {
		return "", fmt.Errorf("invalid libc %q", t.Libc)
	}
	k := newKeyHash()
	k.field("tool", t.Name)
	k.field("version", t.Version)
	k.bytes("lock-entry", t.LockEntry)
	if err := k.own(assets, "shared-tools/build.sh"); err != nil {
		return "", err
	}
	settings, err := miseSettings(assets)
	if err != nil {
		return "", err
	}
	k.bytes("mise-settings", settings)
	k.field("libc", t.Libc)
	if t.Format != "" {
		k.field("format", t.Format)
	}
	return t.Name + "-" + t.Version + "-" + k.sum() + ".erofs", nil
}

// miseSettings is the [settings] table of devvm's shared-tools/mise.toml,
// as JSON (its keys sorted), so only what the settings say counts: not
// comments, how they're written, nor the [tools] table, whose holds the
// lock entry already reflects.
func miseSettings(assets fs.FS) ([]byte, error) {
	const name = "shared-tools/mise.toml"
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		return nil, fmt.Errorf("reading devvm's %s: %w", name, err)
	}
	var config struct {
		Settings map[string]any `toml:"settings"`
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("reading devvm's %s: %w", name, err)
	}
	return json.Marshal(config.Settings)
}

// ParseToolFile reads the tool and version from a tool image's name.
func ParseToolFile(file string) (tool, version string, ok bool) {
	rest, ok := strings.CutSuffix(file, ".erofs")
	if !ok {
		return "", "", false
	}
	i := strings.LastIndex(rest, "-")
	if i < 0 {
		return "", "", false
	}
	tool, version, ok = strings.Cut(rest[:i], "-")
	if !ok || !ToolName(tool) || !ToolVersion(version) {
		return "", "", false
	}
	return tool, version, true
}

// SHA256File is the SHA-256 of a file, in hex.
func SHA256File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // the caller names the file
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
