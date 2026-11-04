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

// ToolKind is what a shared tool is to a VM: a toolchain or a coding agent.
type ToolKind int

const (
	// Toolchain is a language's tools: --tools chooses it, a project's
	// files call for it, and a devcontainer.json's feature of its name
	// (ghcr.io/devcontainers/features/<name>) maps to it.
	Toolchain ToolKind = iota
	// Agent is a coding agent: --agents and devvm setup choose it.
	Agent
)

// Installer is what installs a shared tool in its image, in
// shared-tools/build.sh.
type Installer int

const (
	// Mise installs it from the lock's downloads, whose checksums the lock
	// records per platform.
	Mise Installer = iota
	// Rustup installs it: rustup, pinned in build.sh, checks the toolchain
	// against its own signed manifest, so the lock has no download for it.
	Rustup
)

// ToolDef is a shared tool as devvm offers it: what a VM is given by its
// name.
type ToolDef struct {
	// Name is the name a VM chooses it by, and its policy's file name,
	// shared-tools/<name>.policy.
	Name string
	Kind ToolKind
	// Members are the tools in the lock and in shared-tools/mise.toml
	// that its name stands for, itself first: node is node, pnpm and bun.
	Members []string
	// Detect are the files in a project folder's top level that call for
	// it, in the order they're looked for: the first found is named.
	Detect []string
	// Installer is what installs it.
	Installer Installer
}

// SharedTools are the shared tools, in the fixed order a VM's list holds
// them: the toolchains, then the agents. Every list of shared tools
// derives from it: Toolchains, Agents, GroupTools, what a project's files
// call for, and the devcontainer features devvm maps.
var SharedTools = []ToolDef{
	{Name: "node", Kind: Toolchain, Members: []string{"node", "pnpm", "bun"}, Detect: []string{"package.json", "bun.lock", "bun.lockb"}},
	{Name: "python", Kind: Toolchain, Members: []string{"python", "uv"}, Detect: []string{"pyproject.toml", "requirements.txt", "uv.lock", "setup.py", "Pipfile"}},
	{Name: "rust", Kind: Toolchain, Members: []string{"rust"}, Detect: []string{"Cargo.toml"}, Installer: Rustup},
	{Name: "go", Kind: Toolchain, Members: []string{"go"}, Detect: []string{"go.mod"}},
	{Name: "claude", Kind: Agent, Members: []string{"claude"}},
	{Name: "codex", Kind: Agent, Members: []string{"codex"}},
	{Name: "opencode", Kind: Agent, Members: []string{"opencode"}},
}

// Toolchains are the shared toolchains' names, in SharedTools' order.
var Toolchains = namesOf(Toolchain)

// Agents are the shared coding agents' names, in SharedTools' order,
// after the toolchains.
var Agents = namesOf(Agent)

// namesOf is the names of SharedTools' tools of a kind, in its order.
func namesOf(k ToolKind) []string {
	var out []string
	for _, d := range SharedTools {
		if d.Kind == k {
			out = append(out, d.Name)
		}
	}
	return out
}

// Def returns the shared tool named name, false when none is.
func Def(name string) (ToolDef, bool) {
	for _, d := range SharedTools {
		if d.Name == name {
			return d, true
		}
	}
	return ToolDef{}, false
}

// IsTool reports whether a name is a shared tool's (a toolchain or an
// agent) rather than a layer's.
func IsTool(name string) bool {
	_, ok := Def(name)
	return ok
}

// IsAgent reports whether a name is an agent's.
func IsAgent(name string) bool {
	d, ok := Def(name)
	return ok && d.Kind == Agent
}

// SortTools returns shared tools' names in a VM's order, each once: the
// toolchains in their fixed order, then the agents. Names that aren't
// shared tools are left out.
func SortTools(names []string) []string {
	var out []string
	for _, d := range SharedTools {
		if slices.Contains(names, d.Name) {
			out = append(out, d.Name)
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

// GroupTools returns the tools in the lock behind a shared tool's name,
// its Members: node is node, pnpm and bun; python is python and uv. Any
// other name stands for itself.
func GroupTools(name string) []string {
	if d, ok := Def(name); ok {
		return slices.Clone(d.Members)
	}
	return []string{name}
}

// NoDownload reports whether a tool in the lock needs no download there,
// as its shared tool's installer fetches it: rust, which rustup installs.
func NoDownload(tool string) bool { return installerOf(tool) != Mise }

// installerOf is what installs a tool in the lock: its shared tool's
// Installer, mise for a tool no shared tool has.
func installerOf(tool string) Installer {
	for _, d := range SharedTools {
		if slices.Contains(d.Members, tool) {
			return d.Installer
		}
	}
	return Mise
}

// misePin is the line of layers/base.sh that pins the mise it installs.
var misePin = regexp.MustCompile(`(?m)^mise_version=([0-9A-Za-z._-]+)$`)

// MiseVersion is the version of mise that devvm's layers/base.sh pins
// and installs in base: the mise that installs the shared tools in their
// images, as their build VMs are made from base.
func MiseVersion(assets fs.FS) (string, error) {
	const name = "layers/base.sh"
	data, err := fs.ReadFile(assets, name)
	if err != nil {
		return "", fmt.Errorf("reading devvm's %s: %w", name, err)
	}
	m := misePin.FindAllSubmatch(data, -1)
	if len(m) != 1 {
		return "", fmt.Errorf("devvm's %s pins mise_version %d times, not once", name, len(m))
	}
	return string(m[0][1]), nil
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
	// LockEntry is the tool's entry in the lock, the one its version
	// installs, with its downloads and their checksums: in a canonical
	// form (tools.Lock's Entry), so how mise wrote it doesn't count.
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
// shared-tools/build.sh (in assets, devvm's own files) as its build runs
// it, after the prelude (OwnScript), the [settings] of
// devvm's shared-tools/mise.toml, which change what mise installs, the
// version of mise that installs it (MiseVersion; not for a tool rustup
// installs), the libc and, unless it's build.sh's own, the format.
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
	script, err := OwnScript(assets, "shared-tools/build.sh")
	if err != nil {
		return "", err
	}
	k.bytes("file shared-tools/build.sh", script)
	settings, err := miseSettings(assets)
	if err != nil {
		return "", err
	}
	k.bytes("mise-settings", settings)
	if installerOf(t.Name) == Mise {
		v, err := MiseVersion(assets)
		if err != nil {
			return "", err
		}
		k.field("mise", v)
	}
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
