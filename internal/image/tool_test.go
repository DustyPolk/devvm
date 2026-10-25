package image

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/w3cj/devvm/internal/msg"
)

const nodeEntry = `[[tools.node]]
version = "24.21.0"
backend = "core:node"

[tools.node."platforms.linux-arm64"]
checksum = "sha256:aaaa"
url = "https://nodejs.org/dist/v24.21.0/node-v24.21.0-linux-arm64.tar.xz"
`

func node() Tool {
	return Tool{Name: "node", Version: "24.21.0", LockEntry: []byte(nodeEntry), Libc: "gnu"}
}

func toolRoot(script string) fstest.MapFS {
	return fstest.MapFS{"shared-tools/build.sh": {Data: []byte(script)}}
}

func toolFile(t *testing.T, root fstest.MapFS, tool Tool) string {
	t.Helper()
	f, err := ToolFile(root, tool)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestToolFile(t *testing.T) {
	root := toolRoot("#!/bin/bash\n")
	f := toolFile(t, root, node())
	if !regexp.MustCompile(`^node-24\.21\.0-[0-9a-f]{12}\.erofs$`).MatchString(f) {
		t.Fatalf("got %q", f)
	}
	if again := toolFile(t, toolRoot("#!/bin/bash\n"), node()); again != f {
		t.Errorf("not stable: %q, then %q", f, again)
	}
	tests := []struct {
		name   string
		root   fstest.MapFS
		change func(*Tool)
	}{
		{"the lock entry", root, func(t *Tool) { t.LockEntry = append(t.LockEntry, "# edit\n"...) }},
		{"shared-tools/build.sh", toolRoot("#!/bin/bash\n# edit\n"), func(*Tool) {}},
		{"the libc (an Alpine base)", root, func(t *Tool) { t.Libc = "musl" }},
		{"the version", root, func(t *Tool) { t.Version = "24.22.0" }},
		{"the format (smolvm's)", root, func(t *Tool) { t.Format = "plain-0123456789ab" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := node()
			tt.change(&tool)
			if got := toolFile(t, tt.root, tool); got == f {
				t.Errorf("still %q", got)
			}
		})
	}
}

func TestToolFileRefuses(t *testing.T) {
	root := toolRoot("")
	bad := node()
	bad.Version = "1$(id)"
	_, err := ToolFile(root, bad)
	if m, ok := errors.AsType[msg.M](err); !ok || m.ID() != "tool_bad_version" {
		t.Errorf("version 1$(id): got %v, want tool_bad_version", err)
	}
	for name, change := range map[string]func(*Tool){
		"empty version": func(t *Tool) { t.Version = "" },
		"tool name":     func(t *Tool) { t.Name = "no-de" },
		"libc":          func(t *Tool) { t.Libc = "" },
	} {
		tool := node()
		change(&tool)
		if _, err := ToolFile(root, tool); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := ToolFile(fstest.MapFS{}, node()); err == nil {
		t.Error("no shared-tools/build.sh: no error")
	}
}

func TestParseToolFile(t *testing.T) {
	f := toolFile(t, toolRoot(""), node())
	tests := []struct {
		file, tool, version string
		ok                  bool
	}{
		{f, "node", "24.21.0", true},
		{"rust-1.90.0-rc.1-0123456789ab.erofs", "rust", "1.90.0-rc.1", true},
		{"node-24.21.0-0123456789ab", "", "", false},
		{"node-0123456789ab.erofs", "", "", false},
		{"node.erofs", "", "", false},
		{"node-$(id)-0123456789ab.erofs", "", "", false},
	}
	for _, tt := range tests {
		tool, version, ok := ParseToolFile(tt.file)
		if tool != tt.tool || version != tt.version || ok != tt.ok {
			t.Errorf("%s: got %q %q %v", tt.file, tool, version, ok)
		}
	}
}

func TestSHA256File(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := SHA256File(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := SHA256File(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}
}

// Toolchains in their fixed order, then layers as given, then agents;
// base dropped, and base alone for nothing else.
func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"base":                      "base",
		"base+foo":                  "foo",
		"foo+base+bar":              "foo+bar",
		"foo+bar":                   "foo+bar",
		"claude+python+node":        "node+python+claude",
		"opencode+mytool+go+claude": "go+mytool+claude+opencode",
		"base+claude":               "claude",
		"":                          "", // CheckLayers refuses it
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// An image name's two halves: its image (base and layers) and its shared
// tools; and the tools behind each tool name.
func TestWithoutToolsAndToolsOf(t *testing.T) {
	for in, want := range map[string][2]string{
		"node+claude":         {"base", "node claude"},
		"node+foo+bar+claude": {"foo+bar", "node claude"},
		"base":                {"base", ""},
		"foo":                 {"foo", ""},
	} {
		if got := WithoutTools(in); got != want[0] {
			t.Errorf("WithoutTools(%q) = %q, want %q", in, got, want[0])
		}
		if got := strings.Join(ToolsOf(in), " "); got != want[1] {
			t.Errorf("ToolsOf(%q) = %q, want %q", in, got, want[1])
		}
	}
	for name, want := range map[string]string{"node": "node pnpm bun", "python": "python uv", "go": "go", "claude": "claude"} {
		if got := strings.Join(GroupTools(name), " "); got != want {
			t.Errorf("GroupTools(%q) = %q, want %q", name, got, want)
		}
	}
	if !IsTool("rust") || !IsTool("codex") || IsTool("pnpm") || IsTool("base") || !IsAgent("opencode") || IsAgent("node") {
		t.Error("IsTool or IsAgent is wrong")
	}
}
