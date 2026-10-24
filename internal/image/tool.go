package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/msg"
)

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
}

var (
	toolName = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	// A version as upstream may name one: it goes into file names, mounts
	// and the build VM's commands.
	toolVersion = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]*$`)
)

// ToolFile names a tool's image: <tool>-<version>-<key>.erofs. The key is
// 12 hex digits of a SHA-256 over the tool's lock entry, devvm's
// tools/build.sh (in root) and the libc.
func ToolFile(root fs.FS, t Tool) (string, error) {
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
	if err := k.own(root, "tools/build.sh"); err != nil {
		return "", err
	}
	k.field("libc", t.Libc)
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
