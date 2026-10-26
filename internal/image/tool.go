package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

// Toolchains are the shared toolchains an image name can hold, in the
// fixed order names list them (project.Detect's).
var Toolchains = []string{"node", "python", "rust", "go"}

// Agents are the shared coding agents an image name can hold, listed last.
var Agents = []string{"claude", "codex", "opencode"}

// IsTool reports whether a name in an image name is a shared tool's (a
// toolchain or an agent) rather than a layer's.
func IsTool(name string) bool {
	return slices.Contains(Toolchains, name) || slices.Contains(Agents, name)
}

// IsAgent reports whether a name is an agent's.
func IsAgent(name string) bool { return slices.Contains(Agents, name) }

// GroupTools returns the tools in the lock behind a name image names take:
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

// ToolsOf returns an image name's shared tools, in its order:
// node+foo+claude's are node and claude.
func ToolsOf(name string) []string {
	var out []string
	for p := range strings.SplitSeq(name, "+") {
		if IsTool(p) {
			out = append(out, p)
		}
	}
	return out
}

// Normalize returns an image name's canonical form, so the same layers
// and tools always make one image: the toolchains in their fixed order,
// then the layers bottom-up as given, then the agents, without base, which
// every image is built on; base alone when there's nothing else.
func Normalize(name string) string {
	given := strings.Split(name, "+")
	var parts []string
	for _, t := range Toolchains {
		if slices.Contains(given, t) {
			parts = append(parts, t)
		}
	}
	for _, p := range given {
		if p != Base && !IsTool(p) {
			parts = append(parts, p)
		}
	}
	for _, a := range Agents {
		if slices.Contains(given, a) {
			parts = append(parts, a)
		}
	}
	if len(parts) == 0 {
		return Base
	}
	return strings.Join(parts, "+")
}

// WithoutTools returns the image an image name is built from: base and its
// layers, without the shared tools. node+foo+claude's is foo; node's is
// base.
func WithoutTools(name string) string {
	var parts []string
	for p := range strings.SplitSeq(name, "+") {
		if p != Base && !IsTool(p) {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return Base
	}
	return strings.Join(parts, "+")
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
	toolName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	// A version as upstream may name one: it goes into file names, mounts
	// and the build VM's commands.
	toolVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]*$`)
)

// ToolFile names a tool's image: <tool>-<version>-<key>.erofs. The key is
// 12 hex digits of a SHA-256 over the tool's lock entry, devvm's
// shared-tools/build.sh (in assets, devvm's own files), the libc and,
// unless it's build.sh's own, the format.
func ToolFile(assets fs.FS, t Tool) (string, error) {
	if !toolName.MatchString(t.Name) {
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
	k.field("libc", t.Libc)
	if t.Format != "" {
		k.field("format", t.Format)
	}
	return t.Name + "-" + t.Version + "-" + k.sum() + ".erofs", nil
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
	if !ok || !toolName.MatchString(tool) || !toolVersion.MatchString(version) {
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
